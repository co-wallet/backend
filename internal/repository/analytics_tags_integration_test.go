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

func TestAnalyticsByTag(t *testing.T) {
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
	var account, tag string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,currency,kind,access_mode,initial_balance_date) VALUES ($1,'Wallet','USD','spending','personal',CURRENT_DATE) RETURNING id`, user).Scan(&account))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO tags(user_id,name) VALUES ($1,'Shared tag') RETURNING id`, user).Scan(&tag))
	today := time.Now()
	for _, tx := range []struct {
		kind          string
		amount, share float64
		date          time.Time
		tagged        bool
	}{
		{"expense", 100, 40, today, true}, {"income", 500, 200, today, true},
		{"expense", 50, 25, today, false}, {"income", 250, 100, today, false},
		{"income", 999, 999, today.AddDate(0, -2, 0), true},
	} {
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO transactions(account_id,type,amount,currency,date,created_by) VALUES ($1,$2,$3,'USD',$4,$5) RETURNING id`, account, tx.kind, tx.amount, tx.date, user).Scan(&id))
		exec(`INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES ($1,$2,$3)`, id, user, tx.share)
		if tx.tagged {
			exec(`INSERT INTO transaction_tags(transaction_id,tag_id) VALUES ($1,$2)`, id, tag)
		}
	}
	untaggedTransactions, err := repository.NewTransactionRepository(pool).List(
		ctx,
		user,
		model.TransactionFilter{WithoutTags: true, Page: 1, Limit: 50},
	)
	require.NoError(t, err)
	require.Len(t, untaggedTransactions, 2)

	analytics := repository.NewAnalyticsRepository(pool)
	base := model.AnalyticsFilter{UserID: user, DisplayCurrency: "USD", DateFrom: today.AddDate(0, 0, -1), DateTo: today.AddDate(0, 0, 1)}
	for _, tt := range []struct {
		name   string
		kind   model.TransactionType
		amount float64
	}{
		{"default expenses", "", 40}, {"expenses", model.TransactionTypeExpense, 40}, {"income", model.TransactionTypeIncome, 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := base
			f.TxType = tt.kind
			stats, err := analytics.ByTag(ctx, f)
			require.NoError(t, err)
			untaggedAmount := 25.0
			if tt.kind == model.TransactionTypeIncome {
				untaggedAmount = 100
			}
			require.Equal(t, []model.TagStat{
				{TagID: tag, TagName: "Shared tag", Amount: tt.amount},
				{TagID: "untagged", TagName: "Без тегов", Amount: untaggedAmount},
			}, stats)
			f.AccountIDs = []string{account}
			f.AccountKinds = []model.AccountKind{model.AccountKindSpending}
			f.TagIDs = []string{tag}
			filtered, err := analytics.ByTag(ctx, f)
			require.NoError(t, err)
			require.Equal(t, []model.TagStat{{TagID: tag, TagName: "Shared tag", Amount: tt.amount}}, filtered)
			f.TagIDs = nil
			f.WithoutTags = true
			filtered, err = analytics.ByTag(ctx, f)
			require.NoError(t, err)
			require.Equal(t, []model.TagStat{{TagID: "untagged", TagName: "Без тегов", Amount: untaggedAmount}}, filtered)
			f.AccountKinds = []model.AccountKind{model.AccountKindDeposit}
			filtered, err = analytics.ByTag(ctx, f)
			require.NoError(t, err)
			require.Empty(t, filtered)
			f = base
			f.TxType = tt.kind
			f.UserID = other
			filtered, err = analytics.ByTag(ctx, f)
			require.NoError(t, err)
			require.Empty(t, filtered)
		})
	}
}
