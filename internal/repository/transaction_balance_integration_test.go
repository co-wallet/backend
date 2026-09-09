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

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
	"github.com/co-wallet/backend/internal/repository"
)

// BALANCE_TEST_DATABASE_URL must allow isolated test schemas.
func TestAllTransactionsInBalance(t *testing.T) {
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
	require.NoError(t, goose.UpTo(sqlDB, "../../migrations", 18))
	exec := func(query string, args ...any) {
		_, execErr := pool.Exec(ctx, query, args...)
		require.NoError(t, execErr)
	}

	const u1 = "00000000-0000-0000-0000-000000000001"
	const u2 = "00000000-0000-0000-0000-000000000002"
	exec(`INSERT INTO users (id,username,email,password_hash) VALUES ($1,'one','one@test','unused'),($2,'two','two@test','unused')`, u1, u2)
	var shared, personal, tag string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO accounts (owner_id,name,currency,access_mode,kind,initial_balance,initial_balance_date) VALUES ($1,'Shared','USD','shared','spending',1000,CURRENT_DATE) RETURNING id`, u1).Scan(&shared))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO accounts (owner_id,name,currency,access_mode,kind,initial_balance_date) VALUES ($1,'Personal','USD','personal','spending',CURRENT_DATE) RETURNING id`, u1).Scan(&personal))
	exec(`INSERT INTO account_members (account_id,user_id,default_share) VALUES ($1,$2,0.5),($1,$3,0.5),($4,$2,1)`, shared, u1, u2, personal)
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO tags (user_id,name) VALUES ($1,'test') RETURNING id`, u1).Scan(&tag))
	// Seed both excluded and included operations before dropping the old flag.
	for _, item := range []struct {
		kind     string
		amount   float64
		included bool
	}{
		{"income", 200, false}, {"expense", 40, false}, {"expense", 20, true}, {"transfer", 100, false},
	} {
		var id string
		var toAccount *string
		var toAmount *float64
		if item.kind == "transfer" {
			toAccount = ptr.To(personal)
			toAmount = ptr.To(150.0)
		}
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO transactions (account_id,to_account_id,to_amount,type,amount,currency,date,created_by,include_in_balance) VALUES ($1,$2,$3,$4,$5,'USD',CURRENT_DATE,$6,$7) RETURNING id`, shared, toAccount, toAmount, item.kind, item.amount, u1, item.included).Scan(&id))
		exec(`INSERT INTO transaction_shares (transaction_id,user_id,amount) VALUES ($1,$2,$4),($1,$3,$4)`, id, u1, u2, item.amount/2)
		if item.kind == "expense" {
			exec(`INSERT INTO transaction_tags (transaction_id,tag_id) VALUES ($1,$2)`, id, tag)
		}
	}
	require.NoError(t, goose.Up(sqlDB, "../../migrations"))
	accounts := repository.NewAccountRepository(pool)
	balances, err := accounts.ListBalancesByUser(ctx, u1, "USD")
	require.NoError(t, err)
	require.Equal(t, 520.0, balances[shared].BalanceNative)
	require.Equal(t, 1040.0, balances[shared].TotalNative)
	require.Equal(t, 150.0, balances[personal].BalanceNative)
	analytics := repository.NewAnalyticsRepository(pool)
	f := model.AnalyticsFilter{UserID: u1, DisplayCurrency: "USD", DateFrom: time.Now().AddDate(0, 0, -1), DateTo: time.Now().AddDate(0, 0, 1)}
	summary, err := analytics.Summary(ctx, f)
	require.NoError(t, err)
	require.Equal(t, model.AnalyticsSummary{Balance: 670, Expenses: 30, Income: 100}, summary)
	categories, err := analytics.ByCategory(ctx, f)
	require.NoError(t, err)
	require.Len(t, categories, 1)
	require.Equal(t, 30.0, categories[0].Amount)
	tags, err := analytics.ByTag(ctx, f)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	require.Equal(t, 30.0, tags[0].Amount)
	f.TxType = model.TransactionTypeIncome
	categories, err = analytics.ByCategory(ctx, f)
	require.NoError(t, err)
	require.Len(t, categories, 1)
	require.Equal(t, 100.0, categories[0].Amount)
	// CRUD after the migration catches SQL placeholder and scan mismatches.
	txs := repository.NewTransactionRepository(pool)
	listed, err := txs.List(ctx, u1, model.TransactionFilter{})
	require.NoError(t, err)
	require.Len(t, listed, 4)
	created, err := txs.Create(ctx, model.Transaction{AccountID: personal, Type: model.TransactionTypeExpense, Amount: 10, Currency: "USD", Date: time.Now(), CreatedBy: u1, Shares: []model.TransactionShare{{UserID: u1, Amount: 10}}})
	require.NoError(t, err)
	loaded, err := txs.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, 10.0, loaded.Amount)
	loaded.Amount = 15
	loaded.Shares[0].Amount = 15
	updated, err := txs.Update(ctx, loaded)
	require.NoError(t, err)
	require.Equal(t, 15.0, updated.Amount)
	summary, err = analytics.Summary(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 655.0, summary.Balance)
	require.Equal(t, 45.0, summary.Expenses)
	require.NoError(t, txs.Delete(ctx, created.ID))
	// A rollback restores a usable flag with all operations included.
	require.NoError(t, goose.Down(sqlDB, "../../migrations"))
	var excluded int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE NOT include_in_balance`).Scan(&excluded))
	require.Zero(t, excluded)
}
