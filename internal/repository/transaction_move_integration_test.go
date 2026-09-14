package repository_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
)

func TestMoveTransactionAccount(t *testing.T) {
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

	const editor = "00000000-0000-0000-0000-000000000001"
	const former = "00000000-0000-0000-0000-000000000002"
	exec(`INSERT INTO users(id,username,email,password_hash) VALUES ($1,'editor','editor@test','x'),($2,'former','former@test','x')`, editor, former)
	accounts := repository.NewAccountRepository(pool)
	createAccount := func(name, currency string) model.Account {
		account, err := accounts.Create(ctx, model.Account{OwnerID: editor, Name: name, Currency: currency, AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, InitialBalanceDate: time.Now().AddDate(0, 0, -1)})
		require.NoError(t, err)
		exec(`INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,1)`, account.ID, editor)
		return account
	}
	old := createAccount("Old", "USD")
	target := createAccount("New", "EUR")
	destination := createAccount("Destination", "EUR")
	exec(`UPDATE accounts SET access_mode='shared' WHERE id=$1`, old.ID)
	exec(`UPDATE account_members SET default_share=0.5 WHERE account_id=$1`, old.ID)
	exec(`INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,0.5)`, old.ID, former)
	repo := repository.NewTransactionRepository(pool)
	svc := service.NewTransactionService(pool, repo, accounts, repository.NewTagRepository(pool))
	tx, err := svc.Create(ctx, former, model.CreateTransactionReq{AccountID: old.ID, Type: model.TransactionTypeExpense, Amount: 100, Currency: "USD", Date: time.Now(), Description: ptr.To("Preserved"), Tags: []string{"preserved"}})
	require.NoError(t, err)
	// Failure after updating the row and shares must roll the entire move back.
	_, err = svc.Update(ctx, editor, tx.ID, model.UpdateTransactionReq{AccountID: ptr.To(target.ID), Tags: []string{strings.Repeat("x", 1000)}})
	require.Error(t, err)
	unchanged, err := repo.GetByID(ctx, tx.ID)
	require.NoError(t, err)
	require.Equal(t, old.ID, unchanged.AccountID)
	require.Len(t, unchanged.Shares, 2)
	moved, err := svc.Update(ctx, editor, tx.ID, model.UpdateTransactionReq{AccountID: ptr.To(target.ID)})
	require.NoError(t, err)
	loaded, err := svc.GetByID(ctx, editor, tx.ID)
	require.NoError(t, err)
	require.Equal(t, target.ID, moved.AccountID)
	require.Equal(t, "EUR", loaded.Currency)
	require.Equal(t, target.Name, loaded.Account.Name)
	require.Equal(t, former, loaded.CreatedBy)
	require.Equal(t, "Preserved", *loaded.Description)
	require.Len(t, loaded.Tags, 1)
	require.Len(t, loaded.Shares, 1)
	require.Equal(t, editor, loaded.Shares[0].UserID)
	require.Equal(t, 100.0, loaded.Shares[0].Amount)
	require.False(t, loaded.Shares[0].IsCustom)
	_, err = svc.GetByID(ctx, former, tx.ID)
	require.ErrorIs(t, err, apperr.ErrForbidden)
	balance := func(account string) float64 {
		b, err := accounts.ListBalancesByUser(ctx, editor, "USD")
		require.NoError(t, err)
		return b[account].TotalNative
	}
	require.Equal(t, 0.0, balance(old.ID))
	require.Equal(t, -100.0, balance(target.ID))
	transfer, err := svc.Create(ctx, editor, model.CreateTransactionReq{AccountID: old.ID, ToAccountID: ptr.To(target.ID), Type: model.TransactionTypeTransfer, Amount: 50, Currency: "USD", ToAmount: ptr.To(40.0), Date: time.Now()})
	require.NoError(t, err)
	_, err = svc.Update(ctx, editor, transfer.ID, model.UpdateTransactionReq{AccountID: ptr.To(destination.ID), ToAccountID: ptr.To(old.ID), ToAmount: ptr.To(60.0)})
	require.NoError(t, err)
	loaded, err = svc.GetByID(ctx, editor, transfer.ID)
	require.NoError(t, err)
	require.Equal(t, destination.ID, loaded.AccountID)
	require.Equal(t, old.ID, *loaded.ToAccountID)
	require.Equal(t, "EUR", loaded.Currency)
	require.Equal(t, 60.0, *loaded.ToAmount)
	require.Equal(t, 60.0, balance(old.ID))
	require.Equal(t, -100.0, balance(target.ID))
	require.Equal(t, -50.0, balance(destination.ID))
}
