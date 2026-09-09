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

func TestDashboardTransfers(t *testing.T) {
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
	createAccount := func(owner, name, currency, kind string, share float64) string {
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,currency,kind,access_mode,initial_balance_date) VALUES ($1,$2,$3,$4,'shared',CURRENT_DATE) RETURNING id`, owner, name, currency, kind).Scan(&id))
		exec(`INSERT INTO account_members(account_id,user_id,default_share) VALUES ($1,$2,$3)`, id, owner, share)
		return id
	}
	spending := createAccount(user, "Spending", "USD", "spending", 0.5)
	deposit := createAccount(user, "Deposit", "EUR", "deposit", 0.25)
	external := createAccount(other, "External", "USD", "spending", 1)
	exec(`INSERT INTO exchange_rates(base_currency,quote_currency,rate) VALUES ('USD','EUR',0.5) ON CONFLICT (base_currency,quote_currency) DO UPDATE SET rate = EXCLUDED.rate`)
	var tag string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO tags(user_id,name) VALUES ($1,'test') RETURNING id`, user).Scan(&tag))
	createTransfer := func(source, destination, creator, currency string, amount, received, share float64, date time.Time, tagged bool) {
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO transactions(account_id,to_account_id,type,amount,to_amount,currency,date,created_by) VALUES ($1,$2,'transfer',$3,$4,$5,$6,$7) RETURNING id`, source, destination, amount, received, currency, date, creator).Scan(&id))
		exec(`INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES ($1,$2,$3)`, id, creator, share)
		if tagged {
			exec(`INSERT INTO transaction_tags(transaction_id,tag_id) VALUES ($1,$2)`, id, tag)
		}
	}
	today := time.Now()
	createTransfer(spending, deposit, user, "USD", 100, 40, 30, today, true)
	createTransfer(deposit, spending, user, "EUR", 20, 50, 8, today, false)
	createTransfer(external, spending, other, "USD", 60, 60, 60, today, false)
	createTransfer(spending, external, user, "USD", 10, 10, 4, today, false)
	createTransfer(spending, deposit, user, "USD", 999, 999, 999, today.AddDate(0, -2, 0), false)
	analytics := repository.NewAnalyticsRepository(pool)
	base := model.AnalyticsFilter{UserID: user, DisplayCurrency: "USD", DateFrom: today.AddDate(0, 0, -1), DateTo: today.AddDate(0, 0, 1), IncludeTransferExpenses: true, IncludeTransferIncome: true}
	cases := []struct {
		name            string
		ids             []string
		kinds           []model.AccountKind
		expense, income float64
	}{
		{"all accessible accounts", nil, nil, 4, 30},
		{"source selected", []string{spending}, nil, 34, 55},
		{"destination selected", []string{deposit}, nil, 16, 20},
		{"both selected", []string{spending, deposit}, nil, 4, 30},
		{"spending kind", nil, []model.AccountKind{model.AccountKindSpending}, 34, 55},
		{"deposit kind", nil, []model.AccountKind{model.AccountKindDeposit}, 16, 20},
		{"ids intersect kinds", []string{spending, deposit}, []model.AccountKind{model.AccountKindDeposit}, 16, 20},
		{"inaccessible ids", []string{external}, nil, 0, 0},
		{"empty intersection", []string{deposit}, []model.AccountKind{model.AccountKindSpending}, 0, 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := base
			f.AccountIDs = tt.ids
			f.AccountKinds = tt.kinds
			summary, err := analytics.Summary(ctx, f)
			require.NoError(t, err)
			require.Equal(t, tt.expense, summary.Expenses)
			require.Equal(t, tt.income, summary.Income)
			for _, category := range []struct {
				kind   model.TransactionType
				amount float64
			}{{model.TransactionTypeExpense, tt.expense}, {model.TransactionTypeIncome, tt.income}} {
				f.TxType = category.kind
				stats, err := analytics.ByCategory(ctx, f)
				require.NoError(t, err)
				if category.amount == 0 {
					require.Empty(t, stats)
				} else {
					require.Equal(t, []model.CategoryStat{{CategoryID: "transfers", CategoryName: "Переводы", Amount: category.amount}}, stats)
				}
			}
			for _, flags := range []struct{ expense, income bool }{{false, false}, {true, false}, {false, true}} {
				f.IncludeTransferExpenses = flags.expense
				f.IncludeTransferIncome = flags.income
				toggled, err := analytics.Summary(ctx, f)
				require.NoError(t, err)
				require.Equal(t, summary.Balance, toggled.Balance)
				if flags.expense {
					require.Equal(t, tt.expense, toggled.Expenses)
				} else {
					require.Zero(t, toggled.Expenses)
				}
				if flags.income {
					require.Equal(t, tt.income, toggled.Income)
				} else {
					require.Zero(t, toggled.Income)
				}
			}
		})
	}
	f := base
	f.AccountIDs = []string{spending}
	f.TagIDs = []string{tag}
	filtered, err := analytics.Summary(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 30.0, filtered.Expenses)
	require.Zero(t, filtered.Income)
	f.TagIDs = nil
	f.CategoryIDs = []string{user}
	filtered, err = analytics.Summary(ctx, f)
	require.NoError(t, err)
	require.Zero(t, filtered.Expenses)
	require.Zero(t, filtered.Income)
	f = base
	f.UserID = other
	filtered, err = analytics.Summary(ctx, f)
	require.NoError(t, err)
	require.Equal(t, 60.0, filtered.Expenses)
	require.Equal(t, 10.0, filtered.Income)
	// Deleted accounts leave the selected set but do not erase transfers on active accounts.
	exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, deposit)
	filtered, err = analytics.Summary(ctx, base)
	require.NoError(t, err)
	require.Equal(t, 34.0, filtered.Expenses)
	require.Equal(t, 55.0, filtered.Income)
}
