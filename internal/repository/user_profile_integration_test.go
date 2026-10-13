package repository_test

import (
	"context"
	"fmt"
	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"os"
	"testing"
	"time"
)

func TestUserProfilePersistence(t *testing.T) {
	dsn := os.Getenv("BALANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BALANCE_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := fmt.Sprintf("balance_test_%d", time.Now().UnixNano())
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	require.NoError(t, err)
	defer func() {
		_, cleanupErr := admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		require.NoError(t, cleanupErr)
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	sqlDB := stdlib.OpenDB(*cfg.ConnConfig)
	defer func() { require.NoError(t, sqlDB.Close()) }()
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, "../../migrations"))
	exec := func(query string, args ...any) {
		_, execErr := pool.Exec(ctx, query, args...)
		require.NoError(t, execErr)
	}

	const user = "00000000-0000-0000-0000-000000000001"
	const other = "00000000-0000-0000-0000-000000000002"
	exec(`INSERT INTO users(id,username,email,password_hash) VALUES ($1,'one','one@test','x'),($2,'two','two@test','x')`, user, other)

	repo := repository.NewUserRepository(pool)
	svc := service.NewUserService(repo)
	auth := service.NewAuthService(repo, "profile-test-secret")
	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	require.NoError(t, err)
	exec(`UPDATE users SET password_hash=$1 WHERE id=$2`, string(hash), user)
	initial, err := repo.GetByID(ctx, user)
	require.NoError(t, err)
	_, err = svc.UpdateProfile(ctx, user, service.UpdateProfileReq{Username: ptr.To("new-name"), Email: ptr.To("two@test"), CurrentPassword: "old-password", NewPassword: ptr.To("new-password")})
	require.ErrorIs(t, err, apperr.ErrConflict)
	unchanged, err := repo.GetByID(ctx, user)
	require.NoError(t, err)
	require.Equal(t, initial, unchanged)
	_, err = svc.UpdateProfile(ctx, user, service.UpdateProfileReq{Username: ptr.To("two")})
	require.ErrorIs(t, err, apperr.ErrConflict)
	updated, err := svc.UpdateProfile(ctx, user, service.UpdateProfileReq{Username: ptr.To("new-name"), Email: ptr.To("new@test"), NewPassword: ptr.To("new-password"), CurrentPassword: "old-password"})
	require.NoError(t, err)
	require.Equal(t, user, updated.ID)
	require.Equal(t, "new-name", updated.Username)
	require.Equal(t, "new@test", updated.Email)
	require.Equal(t, initial.DefaultCurrency, updated.DefaultCurrency)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("new-password")))
	_, _, err = auth.Login(ctx, "one@test", "old-password")
	require.ErrorIs(t, err, apperr.ErrUnauthorized)
	_, _, err = auth.Login(ctx, "new@test", "old-password")
	require.ErrorIs(t, err, apperr.ErrUnauthorized)
	loggedIn, _, err := auth.Login(ctx, "new@test", "new-password")
	require.NoError(t, err)
	require.Equal(t, user, loggedIn.ID)
	_, err = repo.UpdateProfile(ctx, user, model.UserProfilePatch{PasswordHash: ptr.To("stale-write")}, string(hash))
	require.ErrorIs(t, err, apperr.ErrConflict)
	otherUser, err := repo.GetByID(ctx, other)
	require.NoError(t, err)
	require.Equal(t, "two", otherUser.Username)
	require.Equal(t, "x", otherUser.PasswordHash)
}
