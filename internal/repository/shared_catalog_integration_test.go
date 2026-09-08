package repository_test

import (
	"context"
	"fmt"
	"os"
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
)

// Run with CATALOG_TEST_DATABASE_URL against a PostgreSQL database where the
// test user can create schemas. Each run uses and removes its own isolated schema.
func TestSharedCatalogIntegration(t *testing.T) {
	dsn := os.Getenv("CATALOG_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CATALOG_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := fmt.Sprintf("catalog_test_%d", time.Now().UnixNano())
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
	require.NoError(t, goose.UpTo(sqlDB, "../../migrations", 17))
	exec := func(query string, args ...any) {
		_, execErr := pool.Exec(ctx, query, args...)
		require.NoError(t, execErr)
	}
	const u1 = "00000000-0000-0000-0000-000000000001"
	const u2 = "00000000-0000-0000-0000-000000000002"
	const c1 = "00000000-0000-0000-0000-000000000011"
	const c2 = "00000000-0000-0000-0000-000000000012"
	const t1 = "00000000-0000-0000-0000-000000000021"
	const t2 = "00000000-0000-0000-0000-000000000022"
	exec(`INSERT INTO users (id, username, email, password_hash) VALUES ($1, 'one', 'one@test', 'unused'), ($2, 'two', 'two@test', 'unused')`, u1, u2)
	exec(`INSERT INTO categories (id,user_id,name,type) VALUES ($1,$2,'Food','expense'), ($3,$4,'food','expense')`, c1, u1, c2, u2)
	exec(`UPDATE categories SET deleted_at=now() WHERE id=$1`, c2)
	exec(`INSERT INTO tags (id,user_id,name) VALUES ($1,$2,'travel'), ($3,$4,'travel')`, t1, u1, t2, u2)
	var accountID, txID string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO accounts (owner_id,name,currency,access_mode,kind,initial_balance_date) VALUES ($1,'Test','USD','personal','spending',CURRENT_DATE) RETURNING id`, u1).Scan(&accountID))
	exec(`INSERT INTO account_members (account_id,user_id,default_share) VALUES ($1,$2,1)`, accountID, u1)
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO transactions (account_id,type,amount,currency,category_id,date,created_by) VALUES ($1,'expense',100,'USD',$2,CURRENT_DATE,$3) RETURNING id`, accountID, c2, u1).Scan(&txID))
	exec(`INSERT INTO transaction_shares (transaction_id,user_id,amount) VALUES ($1,$2,100)`, txID, u1)
	exec(`INSERT INTO transaction_tags (transaction_id,tag_id) VALUES ($1,$2),($1,$3)`, txID, t1, t2)
	require.NoError(t, goose.Up(sqlDB, "../../migrations"))

	cats := repository.NewCategoryRepository(pool)
	tags := repository.NewTagRepository(pool)
	listed, err := cats.ListByUser(ctx, u2, model.CategoryTypeExpense)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, c1, listed[0].ID)
	require.True(t, listed[0].Hidden)
	cat, err := cats.GetByID(ctx, c1, u1)
	require.NoError(t, err)
	require.False(t, cat.Hidden)
	cat.Name = "Groceries"
	cat.Icon = ptr.To("preset:basket|green|none")
	_, err = cats.Update(ctx, cat)
	require.NoError(t, err)
	_, err = cats.Create(ctx, model.Category{UserID: u2, Name: "groceries", Type: model.CategoryTypeExpense})
	require.ErrorIs(t, err, apperr.ErrConflict)
	require.NoError(t, cats.SetHidden(ctx, c1, u1, true))
	require.NoError(t, cats.SetHidden(ctx, c1, u1, true))
	require.NoError(t, cats.SetHidden(ctx, c1, u2, false))
	cat, err = cats.GetByID(ctx, c1, u2)
	require.NoError(t, err)
	require.False(t, cat.Hidden)
	require.Equal(t, "Groceries", cat.Name)
	require.ErrorIs(t, cats.HardDelete(ctx, c1, u2), apperr.ErrConflict)

	tag, err := tags.GetByID(ctx, t1, u2)
	require.NoError(t, err)
	tag.Name = "holiday"
	_, err = tags.Update(ctx, tag)
	require.NoError(t, err)
	require.NoError(t, tags.SetHidden(ctx, t1, u1, true))
	require.NoError(t, tags.SetHidden(ctx, t1, u1, true))
	require.ErrorIs(t, tags.Delete(ctx, t1, u2), apperr.ErrConflict)
	ownerTags, err := tags.ListByUser(ctx, u1, "hol")
	require.NoError(t, err)
	require.Len(t, ownerTags, 1)
	require.True(t, ownerTags[0].Hidden)
	require.Equal(t, 1, ownerTags[0].TxCount)
	otherTags, err := tags.ListByUser(ctx, u2, "")
	require.NoError(t, err)
	require.Len(t, otherTags, 1)
	require.False(t, otherTags[0].Hidden)
	require.Zero(t, otherTags[0].TxCount)
	linked, err := tags.ListForTransaction(ctx, txID)
	require.NoError(t, err)
	require.Len(t, linked, 1)
	require.Equal(t, "holiday", linked[0].Name)
	reused, err := tags.UpsertForTransaction(ctx, txID, u2, []string{"HOLIDAY"})
	require.NoError(t, err)
	require.Equal(t, t1, reused[0].ID)
	require.Equal(t, "holiday", reused[0].Name)

	txs, err := repository.NewTransactionRepository(pool).List(ctx, u1, model.TransactionFilter{CategoryIDs: []string{c1}, TagIDs: []string{t1}, Page: 1, Limit: 50})
	require.NoError(t, err)
	require.Len(t, txs, 1)
	require.Equal(t, txID, txs[0].ID)
	filter := model.AnalyticsFilter{UserID: u1, DisplayCurrency: "USD", TxType: model.TransactionTypeExpense, DateFrom: time.Now().AddDate(0, 0, -1), DateTo: time.Now().AddDate(0, 0, 1), CategoryIDs: []string{c1}, TagIDs: []string{t1}}
	stats, err := repository.NewAnalyticsRepository(pool).ByCategory(ctx, filter)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "Groceries", stats[0].CategoryName)
	require.Equal(t, 100.0, stats[0].Amount)
	tagStats, err := repository.NewAnalyticsRepository(pool).ByTag(ctx, filter)
	require.NoError(t, err)
	require.Len(t, tagStats, 1)
	require.Equal(t, "holiday", tagStats[0].TagName)

	exec(`DELETE FROM transactions WHERE id=$1`, txID)
	require.NoError(t, tags.Delete(ctx, t1, u2))
	require.NoError(t, cats.HardDelete(ctx, c1, u2))
	_, err = cats.GetByID(ctx, c1, u1)
	require.ErrorIs(t, err, apperr.ErrNotFound)
	_, err = tags.GetByID(ctx, t1, u1)
	require.ErrorIs(t, err, apperr.ErrNotFound)
}
