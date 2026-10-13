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
	"github.com/co-wallet/backend/internal/repository"
)

func TestTransactionsWithoutCategory(t *testing.T) {
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
	var account, category string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,currency,kind,access_mode,initial_balance_date) VALUES ($1,'Wallet','USD','spending','personal',CURRENT_DATE) RETURNING id`, user).Scan(&account))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO categories(user_id,name,type) VALUES ($1,'Food','expense') RETURNING id`, user).Scan(&category))
	today := time.Now()
	for _, tx := range []struct {
		kind     string
		amount   float64
		category *string
	}{
		{"expense", 10, nil}, {"income", 20, nil}, {"expense", 100, &category},
	} {
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO transactions(account_id,type,amount,currency,date,created_by,category_id) VALUES ($1,$2,$3,'USD',$4,$5,$6) RETURNING id`, account, tx.kind, tx.amount, today, user, tx.category).Scan(&id))
		exec(`INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES ($1,$2,$3)`, id, user, tx.amount)
	}
	var destination, transferID string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,currency,kind,access_mode,initial_balance_date) VALUES ($1,'Destination','USD','spending','personal',CURRENT_DATE) RETURNING id`, other).Scan(&destination))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO transactions(account_id,to_account_id,type,amount,currency,date,created_by) VALUES ($1,$2,'transfer',999,'USD',$3,$4) RETURNING id`, account, destination, today, user).Scan(&transferID))
	exec(`INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES ($1,$2,999)`, transferID, user)
	list := repository.NewTransactionRepository(pool)
	all, err := list.List(ctx, user, model.TransactionFilter{Page: 1, Limit: 50})
	require.NoError(t, err)
	require.Len(t, all, 4)
	txs, err := list.List(ctx, user, model.TransactionFilter{WithoutCategory: true, Page: 1, Limit: 50})
	require.NoError(t, err)
	require.Len(t, txs, 2)
	for _, tx := range txs {
		require.Nil(t, tx.CategoryID)
		require.NotEqual(t, model.TransactionTypeTransfer, tx.Type)
	}
	transfers, err := list.List(ctx, user, model.TransactionFilter{WithoutCategory: true, Types: []model.TransactionType{model.TransactionTypeTransfer}, Page: 1, Limit: 50})
	require.NoError(t, err)
	require.Empty(t, transfers)
	filtered, err := list.List(ctx, user, model.TransactionFilter{WithoutCategory: true, Types: []model.TransactionType{model.TransactionTypeIncome}, Page: 1, Limit: 50})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, 20.0, filtered[0].Amount)
	forbidden, err := list.List(ctx, other, model.TransactionFilter{WithoutCategory: true, Page: 1, Limit: 50})
	require.NoError(t, err)
	require.Empty(t, forbidden)
	analytics := repository.NewAnalyticsRepository(pool)
	f := model.AnalyticsFilter{UserID: user, DisplayCurrency: "USD", DateFrom: today.AddDate(0, 0, -1), DateTo: today.AddDate(0, 0, 1), WithoutCategory: true}
	f.IncludeTransferExpenses = true
	f.IncludeTransferIncome = true
	summary, err := analytics.Summary(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 10.0, summary.Expenses)
	require.Equal(t, 20.0, summary.Income)
	f.TxType = model.TransactionTypeExpense
	stats, err := analytics.ByCategory(ctx, f)
	require.NoError(t, err)
	require.Equal(t, []model.CategoryStat{{CategoryID: "uncategorized", CategoryName: "Без категории", Amount: 10}}, stats)
}
