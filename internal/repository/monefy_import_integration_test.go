package repository_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/importer/preview"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

type importFixture struct {
	pool                *pgxpool.Pool
	store               *preview.Store
	svc                 *service.ImportService
	repo                *repository.ImportRepository
	user, other, source string
}

func newImportFixture(t *testing.T) *importFixture {
	t.Helper()
	dsn := os.Getenv("MONEFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MONEFY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("import_test_%d", time.Now().UnixNano())
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	require.NoError(t, err)
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Close()
		_, e := admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		require.NoError(t, e)
		admin.Close()
	})
	d := stdlib.OpenDB(*cfg.ConnConfig)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(d, "../../migrations"))
	require.NoError(t, d.Close())
	store, err := preview.New(t.TempDir())
	require.NoError(t, err)
	f := &importFixture{pool: pool, store: store, repo: repository.NewImportRepository(pool), user: uuid.NewString(), other: uuid.NewString(), source: filepath.Join(t.TempDir(), "source.db")}
	f.svc = service.NewImportService(pool, f.repo, store)
	_, err = pool.Exec(ctx, `INSERT INTO users(id,username,email,password_hash) VALUES($1,'importer','importer@test','unused'),($2,'other','other@test','unused');`, f.user, f.other)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO currencies(code,name) VALUES('TRY','Lira')`)
	require.NoError(t, err)
	fixture, err := os.ReadFile("../importer/monefy/testdata/v11.sql")
	require.NoError(t, err)
	source, err := sql.Open("sqlite", f.source)
	require.NoError(t, err)
	_, err = source.Exec(string(fixture))
	require.NoError(t, err)
	_, err = source.Exec(`UPDATE Account SET InitialBalanceCents=1000 WHERE Id='travel'`)
	require.NoError(t, err)
	require.NoError(t, source.Close())
	return f
}
func (f *importFixture) configured(t *testing.T) model.ImportPreview {
	t.Helper()
	src, err := os.Open(f.source)
	require.NoError(t, err)
	defer src.Close() //nolint:errcheck
	p, err := f.svc.Preview(context.Background(), f.user, src)
	require.NoError(t, err)
	p, err = f.svc.Configure(context.Background(), f.user, p.ID, map[string]model.AccountKind{"cash": "spending", "travel": "deposit", "reserve": "investment"}, nil, nil)
	require.NoError(t, err)
	require.True(t, p.Report.CanImport(), p.Report.Diagnostics)
	return p
}

func TestMonefyAtomicImportAndRepeat(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	var category string
	require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO categories(user_id,name,type,icon) VALUES($1,'FOOD','expense','original') RETURNING id`, f.other).Scan(&category))
	a, err := f.svc.Availability(ctx, f.user)
	require.NoError(t, err)
	require.Empty(t, a.Reasons)
	p := f.configured(t)
	require.Equal(t, category, p.Categories[0].ExistingID)
	_, err = f.svc.Confirm(ctx, f.other, p.ID, true)
	require.ErrorIs(t, err, apperr.ErrNotFound)
	var results [2]model.ImportResult
	var errs [2]error
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results[i], errs[i] = f.svc.Confirm(ctx, f.user, p.ID, false) }()
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, results[0], results[1])
	require.Equal(t, 1, results[0].ReusedCategories)
	var txCount, shareCount, memberCount int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM transactions),(SELECT count(*) FROM transaction_shares),(SELECT count(*) FROM account_members WHERE default_share=1)`).Scan(&txCount, &shareCount, &memberCount))
	require.Equal(t, 5, txCount)
	require.Equal(t, 5, shareCount)
	require.Equal(t, 3, memberCount)
	var wrong bool
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM transaction_shares s JOIN transactions t ON t.id=s.transaction_id WHERE s.user_id<>$1 OR s.amount<>t.amount)`, f.user).Scan(&wrong))
	require.False(t, wrong)
	var debit, credit string
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT amount::text,to_amount::text FROM transactions WHERE type='transfer' AND to_amount<>amount`).Scan(&debit, &credit))
	require.Equal(t, "1234.5670", debit)
	require.Equal(t, "411.5210", credit)
	var owner, icon string
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT user_id,icon FROM categories WHERE id=$1`, category).Scan(&owner, &icon))
	require.Equal(t, f.other, owner)
	require.Equal(t, "original", icon)
	balances, err := repository.NewAccountRepository(f.pool).ListBalancesByUser(ctx, f.user, "RUB")
	require.NoError(t, err)
	accounts, err := repository.NewAccountRepository(f.pool).ListByUser(ctx, f.user)
	require.NoError(t, err)
	for _, account := range accounts {
		for i, source := range p.Report.Accounts {
			if account.Name == source.Name {
				require.Equal(t, p.Accounts[i].Balance, fmt.Sprintf("%.3f", balances[account.ID].BalanceNative))
			}
		}
	}
	_, err = f.store.Load(f.user, p.ID)
	require.ErrorIs(t, err, apperr.ErrNotFound)
	result, err := f.svc.Confirm(ctx, f.user, p.ID, false)
	require.NoError(t, err)
	require.Equal(t, results[0], result)
}

func TestMonefyRollback(t *testing.T) {
	f := newImportFixture(t)
	p := f.configured(t)
	ctx := context.Background()
	// Fail late, after accounts, categories and ordinary transactions were inserted.
	_, err := f.pool.Exec(ctx, `ALTER TABLE transactions ADD CONSTRAINT reject_transfer CHECK(type<>'transfer')`)
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, false)
	require.Error(t, err)
	var count int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM accounts)+(SELECT count(*) FROM categories)+(SELECT count(*) FROM transactions)+(SELECT count(*) FROM transaction_shares)+(SELECT count(*) FROM account_members)+(SELECT count(*) FROM monefy_imports)`).Scan(&count))
	require.Zero(t, count)
	_, err = f.store.Load(f.user, p.ID)
	require.NoError(t, err)
	_, err = f.pool.Exec(ctx, `ALTER TABLE transactions DROP CONSTRAINT reject_transfer`)
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, false)
	require.NoError(t, err)
}

func TestMonefyExclusionsAndStaleCatalog(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	source, err := sql.Open("sqlite", f.source)
	require.NoError(t, err)
	_, err = source.Exec(`INSERT INTO "Transaction" VALUES ('orphan','food','deleted',1000,638397504000000000,NULL,NULL,NULL)`)
	require.NoError(t, err)
	require.NoError(t, source.Close())
	src, err := os.Open(f.source)
	require.NoError(t, err)
	p, err := f.svc.Preview(ctx, f.user, src)
	require.NoError(t, err)
	require.NoError(t, src.Close())
	p, err = f.svc.Configure(ctx, f.user, p.ID, map[string]model.AccountKind{"cash": "spending", "travel": "deposit", "reserve": "investment"}, nil, nil)
	require.NoError(t, err)
	require.Len(t, p.Report.Exclusions, 1)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, false)
	require.ErrorContains(t, err, "exclusions_not_confirmed")
	_, err = f.pool.Exec(ctx, `INSERT INTO categories(user_id,name,type) VALUES($1,'Food','expense')`, f.other)
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, true)
	require.ErrorContains(t, err, "catalog_changed")
	p, err = f.svc.Configure(ctx, f.user, p.ID, map[string]model.AccountKind{"cash": "spending", "travel": "deposit", "reserve": "investment"}, nil, nil)
	require.NoError(t, err)
	result, err := f.svc.Confirm(ctx, f.user, p.ID, true)
	require.NoError(t, err)
	require.Equal(t, 3, result.Transactions)
	var count int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE name='Deleted'`).Scan(&count))
	require.Zero(t, count)
}

func TestMonefyEmptinessCannotBeHidden(t *testing.T) {
	for _, kind := range []string{"deleted_account", "hidden_category", "membership", "transaction_share"} {
		t.Run(kind, func(t *testing.T) {
			f := newImportFixture(t)
			p := f.configured(t)
			ctx := context.Background()
			var err error
			switch kind {
			case "deleted_account":
				_, err = f.pool.Exec(ctx, `INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance_date,deleted_at) VALUES($1,'Deleted','RUB','personal','spending',CURRENT_DATE,now())`, f.user)
			case "hidden_category":
				_, err = f.pool.Exec(ctx, `WITH c AS (INSERT INTO categories(user_id,name,type) VALUES($1,'Hidden','expense') RETURNING id) INSERT INTO hidden_categories(user_id,category_id) SELECT $1,id FROM c`, f.user)
			case "membership":
				_, err = f.pool.Exec(ctx, `WITH a AS (INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance_date) VALUES($1,'Shared','RUB','shared','spending',CURRENT_DATE) RETURNING id) INSERT INTO account_members(account_id,user_id,default_share) SELECT id,$2,0.5 FROM a`, f.other, f.user)
			case "transaction_share":
				_, err = f.pool.Exec(ctx, `WITH a AS (INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance_date) VALUES($1,'Other','RUB','personal','spending',CURRENT_DATE) RETURNING id), t AS (INSERT INTO transactions(account_id,type,amount,currency,date,created_by) SELECT id,'income',1,'RUB',CURRENT_DATE,$1 FROM a RETURNING id) INSERT INTO transaction_shares(transaction_id,user_id,amount) SELECT id,$2,1 FROM t`, f.other, f.user)
			}
			require.NoError(t, err)
			a, err := f.svc.Availability(ctx, f.user)
			require.NoError(t, err)
			require.NotEmpty(t, a.Reasons)
			_, err = f.svc.Confirm(ctx, f.user, p.ID, false)
			require.ErrorContains(t, err, "account_not_empty")
		})
	}
}

func TestMonefyLocksOrdinaryWrites(t *testing.T) {
	for _, kind := range []string{"account", "category", "membership"} {
		t.Run(kind, func(t *testing.T) {
			f := newImportFixture(t)
			ctx := context.Background()
			p := f.configured(t)
			var account string
			require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance_date) VALUES($1,'Other','RUB','shared','spending',CURRENT_DATE) RETURNING id`, f.other).Scan(&account))
			query := `INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance_date) VALUES($1,'Ordinary','RUB','personal','spending',CURRENT_DATE)`
			args := []any{f.user}
			if kind == "category" {
				query = `INSERT INTO categories(user_id,name,type) VALUES($1,'Ordinary','income')`
			}
			if kind == "membership" {
				query = `INSERT INTO account_members(account_id,user_id,default_share) VALUES($2,$1,0.5)`
				args = append(args, account)
			}
			// Import owns the user lock first: ordinary FK writes cannot commit through it.
			locked, err := f.pool.Begin(ctx)
			require.NoError(t, err)
			defer locked.Rollback(ctx) //nolint:errcheck // Закрываем транзакцию и при ошибке assertion.
			require.NoError(t, f.repo.WithTx(locked).LockUser(ctx, f.user))
			writer, err := f.pool.Begin(ctx)
			require.NoError(t, err)
			defer writer.Rollback(ctx) //nolint:errcheck // Страховочный откат при ошибке теста.
			_, err = writer.Exec(ctx, `SET LOCAL lock_timeout='150ms'`)
			require.NoError(t, err)
			_, err = writer.Exec(ctx, query, args...)
			require.Error(t, err)
			require.Contains(t, err.Error(), "lock timeout")
			require.NoError(t, writer.Rollback(ctx))
			require.NoError(t, locked.Rollback(ctx))
			// Ordinary operation owns its FK lock first: confirm must wait and then
			// observe the committed data, instead of using an earlier snapshot.
			ordinary, err := f.pool.Begin(ctx)
			require.NoError(t, err)
			defer ordinary.Rollback(ctx) //nolint:errcheck // Закрываем транзакцию и при ошибке assertion.
			_, err = ordinary.Exec(ctx, query, args...)
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() { _, e := f.svc.Confirm(ctx, f.user, p.ID, false); done <- e }()
			select {
			case e := <-done:
				t.Fatalf("confirm bypassed ordinary write: %v", e)
			case <-time.After(150 * time.Millisecond):
			}
			require.NoError(t, ordinary.Commit(ctx))
			select {
			case e := <-done:
				require.ErrorContains(t, e, "account_not_empty")
			case <-time.After(5 * time.Second):
				t.Fatal("confirm did not finish")
			}
		})
	}
}

func TestMonefyCategoryIconsPersistOnImport(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	p := f.configured(t)
	kinds := map[string]model.AccountKind{"cash": "spending", "travel": "deposit", "reserve": "investment"}
	chosen := "preset:groceries|purple|none"
	category := p.Categories[0]
	configured, err := f.svc.Configure(ctx, f.user, p.ID, kinds, map[string]string{category.SourceID: chosen}, map[string]string{"cash": "preset:wallet|pink|none"})
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, configured.ID, true)
	require.NoError(t, err)
	var icon string
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT icon FROM categories WHERE user_id=$1 AND name=$2`, f.user, category.Name).Scan(&icon))
	require.Equal(t, chosen, icon)
	for _, account := range configured.Accounts {
		var savedIcon string
		var name string
		for _, source := range configured.Report.Accounts {
			if source.ID == account.SourceID {
				name = source.Name
			}
		}
		require.NoError(t, f.pool.QueryRow(ctx, `SELECT icon FROM accounts WHERE owner_id=$1 AND name=$2`, f.user, name).Scan(&savedIcon))
		require.Equal(t, account.Icon, savedIcon)
	}
	var nonPreset int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE owner_id=$1 AND icon NOT LIKE 'preset:%'`, f.user).Scan(&nonPreset))
	require.Zero(t, nonPreset)
}
