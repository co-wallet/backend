package repository_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/stretchr/testify/require"
)

func (f *importFixture) oldAccount(t *testing.T, owner string) string {
	t.Helper()
	var id string
	require.NoError(t, f.pool.QueryRow(context.Background(), `INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance_date) VALUES($1,'Old','RUB','personal','spending',CURRENT_DATE) RETURNING id`, owner).Scan(&id))
	return id
}
func (f *importFixture) oldHistory(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	a := f.oldAccount(t, f.user)
	var tx string
	require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO transactions(account_id,type,amount,currency,date,created_by,description) VALUES($1,'expense',42,'RUB',CURRENT_DATE,$2,'old description') RETURNING id`, a, f.user).Scan(&tx))
	for _, q := range []string{
		`INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,1)`,
		`UPDATE accounts SET deleted_at=now() WHERE id=$1 AND owner_id=$2`,
	} {
		_, err := f.pool.Exec(ctx, q, a, f.user)
		require.NoError(t, err)
	}
	_, err := f.pool.Exec(ctx, `INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES($1,$2,42)`, tx, f.user)
	require.NoError(t, err)
	return a, tx
}
func (f *importFixture) replacement(t *testing.T) model.ImportPreview {
	t.Helper()
	src, err := os.Open(f.source)
	require.NoError(t, err)
	p, err := f.svc.Preview(context.Background(), f.user, src, model.ImportReplace)
	require.NoError(t, src.Close())
	require.NoError(t, err)
	return p
}
func (f *importFixture) exists(t *testing.T, account, tx string) {
	t.Helper()
	var exists bool
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1) AND EXISTS(SELECT 1 FROM transactions WHERE id=$2)`, account, tx).Scan(&exists))
	require.True(t, exists)
}
func TestMonefyReplacementAtomicAndIdempotent(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	a, tx := f.oldHistory(t)
	other := f.oldAccount(t, f.other)
	var category, tag string
	require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO categories(user_id,name,type,icon) VALUES($1,'FOOD','expense','preset:cafe') RETURNING id`, f.user).Scan(&category))
	require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO tags(user_id,name) VALUES($1,'retained') RETURNING id`, f.user).Scan(&tag))
	for _, q := range []string{
		`INSERT INTO hidden_categories(user_id,category_id) VALUES($1,$2)`,
		`INSERT INTO transactions(created_by,category_id,account_id,type,amount,currency,date) VALUES($1,$2,$3,'expense',1,'RUB',CURRENT_DATE)`,
	} {
		args := []any{f.other, category}
		if strings.Contains(q, "account_id") {
			args = append(args, other)
		}
		_, err := f.pool.Exec(ctx, q, args...)
		require.NoError(t, err)
	}
	_, err := f.pool.Exec(ctx, `UPDATE transactions SET category_id=$1 WHERE id=$2`, category, tx)
	require.NoError(t, err)
	_, err = f.pool.Exec(ctx, `INSERT INTO transaction_tags(transaction_id,tag_id) VALUES($1,$2)`, tx, tag)
	require.NoError(t, err)
	_, err = f.pool.Exec(ctx, `INSERT INTO hidden_tags(user_id,tag_id) VALUES($1,$2)`, f.user, tag)
	require.NoError(t, err)
	// Compare the complete profile and shared catalog rows before/after.
	var before string
	query := `SELECT jsonb_build_array((SELECT to_jsonb(u) FROM users u WHERE id=$1),(SELECT to_jsonb(c) FROM categories c WHERE id=$2),(SELECT to_jsonb(t) FROM tags t WHERE id=$3),(SELECT jsonb_agg(h) FROM hidden_tags h),(SELECT jsonb_agg(h) FROM hidden_categories h))::text`
	require.NoError(t, f.pool.QueryRow(ctx, query, f.user, category, tag).Scan(&before))
	p := f.replacement(t)
	require.True(t, p.Report.CanImport(), p.Report.Diagnostics)
	require.Equal(t, map[string]int{"accounts": 1, "deleted_accounts": 1, "transactions": 1, "transfers": 0, "members": 1, "shares": 1, "tag_links": 1}, p.Replacement.Counts)
	require.Equal(t, a, p.Replacement.Accounts[0].ID)
	require.NotNil(t, p.Replacement.Accounts[0].DeletedAt)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, true, false)
	require.ErrorContains(t, err, "deletion_not_confirmed")
	f.exists(t, a, tx)
	var results [2]model.ImportResult
	var errs [2]error
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results[i], errs[i] = f.svc.Confirm(ctx, f.user, p.ID, true, true) }()
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, results[0], results[1])
	var count int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM accounts WHERE id=$1)+(SELECT count(*) FROM transactions WHERE id=$2)+(SELECT count(*) FROM transaction_shares WHERE transaction_id=$2)+(SELECT count(*) FROM transaction_tags WHERE transaction_id=$2)+(SELECT count(*) FROM account_members WHERE account_id=$1)`, a, tx).Scan(&count))
	require.Zero(t, count)
	var after string
	require.NoError(t, f.pool.QueryRow(ctx, query, f.user, category, tag).Scan(&after))
	require.Equal(t, before, after)
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE account_id=$1`, other).Scan(&count))
	require.Equal(t, 1, count)
	// Even after a later replacement, retrying an old receipt cannot delete new data.
	next := f.replacement(t)
	_, err = f.svc.Confirm(ctx, f.user, next.ID, true, true)
	require.NoError(t, err)
	scope, err := f.repo.Replacement(ctx, f.user)
	require.NoError(t, err)
	repeated, err := f.svc.Confirm(ctx, f.user, p.ID, false, false)
	require.NoError(t, err)
	require.Equal(t, results[0], repeated)
	afterScope, err := f.repo.Replacement(ctx, f.user)
	require.NoError(t, err)
	require.Equal(t, scope, afterScope)
}
func TestMonefyReplacementRollbackAndBadSource(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	a, tx := f.oldHistory(t)
	_, err := f.svc.Preview(ctx, f.user, strings.NewReader("bad sqlite"), model.ImportReplace)
	require.ErrorContains(t, err, "source_")
	f.exists(t, a, tx)
	p := f.replacement(t)
	before, err := f.repo.Replacement(ctx, f.user)
	require.NoError(t, err)
	_, err = f.pool.Exec(ctx, `ALTER TABLE transactions ADD CONSTRAINT fail_import CHECK(type<>'transfer')`)
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, true, true)
	require.Error(t, err)
	after, err := f.repo.Replacement(ctx, f.user)
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, err = f.repo.Receipt(ctx, f.user, p.ID)
	require.ErrorIs(t, err, apperr.ErrNotFound)
	_, err = f.pool.Exec(ctx, `ALTER TABLE transactions DROP CONSTRAINT fail_import`)
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, true, true)
	require.NoError(t, err)
}
func TestMonefyReplacementBlocksExternalRelations(t *testing.T) {
	for _, kind := range []string{"foreign_members", "outgoing", "incoming", "outgoing_shared", "incoming_shared", "foreign_authors", "foreign_shares"} {
		t.Run(kind, func(t *testing.T) {
			f := newImportFixture(t)
			ctx := context.Background()
			a, tx := f.oldHistory(t)
			other := f.oldAccount(t, f.other)
			var err error
			switch kind {
			case "foreign_members":
				_, err = f.pool.Exec(ctx, `INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,0)`, a, f.other)
			case "outgoing", "outgoing_shared":
				_, err = f.pool.Exec(ctx, `UPDATE transactions SET type='transfer',to_account_id=$1 WHERE id=$2`, other, tx)
			case "incoming", "incoming_shared":
				_, err = f.pool.Exec(ctx, `UPDATE transactions SET type='transfer',account_id=$1,to_account_id=$2 WHERE id=$3`, other, a, tx)
			case "foreign_authors":
				_, err = f.pool.Exec(ctx, `UPDATE transactions SET created_by=$1 WHERE id=$2`, f.other, tx)
			case "foreign_shares":
				_, err = f.pool.Exec(ctx, `INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES($1,$2,0)`, tx, f.other)
			}
			require.NoError(t, err)
			if strings.HasSuffix(kind, "_shared") {
				_, err = f.pool.Exec(ctx, `UPDATE accounts SET owner_id=$1,access_mode='shared' WHERE id=$2`, f.user, other)
				require.NoError(t, err)
			}
			p := f.replacement(t)
			require.False(t, p.Report.CanImport())
			require.NotEmpty(t, p.Report.Diagnostics)
			code := kind
			if strings.HasPrefix(kind, "incoming") || strings.HasPrefix(kind, "outgoing") {
				code = "external_transactions"
			}
			require.Positive(t, p.Replacement.Blockers[code])
			_, err = f.svc.Confirm(ctx, f.user, p.ID, true, true)
			require.ErrorContains(t, err, "preview_blocked")
			f.exists(t, a, tx)
		})
	}
}

func TestMonefyReplacementPreservesSharedHistory(t *testing.T) {
	for _, kind := range []string{"owned", "archived", "joined", "only_shared"} {
		t.Run(kind, func(t *testing.T) {
			f := newImportFixture(t)
			ctx := context.Background()
			a, tx := f.oldHistory(t)
			owner := f.user
			if kind == "joined" {
				owner = f.other
			}
			shared := f.oldAccount(t, owner)
			_, err := f.pool.Exec(ctx, `UPDATE accounts SET access_mode='shared',name='Cash' WHERE id=$1`, shared)
			require.NoError(t, err)
			if kind == "archived" {
				_, err = f.pool.Exec(ctx, `UPDATE accounts SET deleted_at=now() WHERE id=$1`, shared)
				require.NoError(t, err)
			}
			if kind == "only_shared" {
				_, err = f.pool.Exec(ctx, `UPDATE accounts SET access_mode='shared' WHERE id=$1`, a)
				require.NoError(t, err)
			}
			_, err = f.pool.Exec(ctx, `INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,0.5),($1,$3,0.5)`, shared, f.user, f.other)
			require.NoError(t, err)
			// Both authors and both shares must survive, including the importing user's.
			for _, author := range []string{f.user, f.other} {
				var id string
				require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO transactions(account_id,type,amount,currency,date,created_by) VALUES($1,'expense',10,'RUB',CURRENT_DATE,$2) RETURNING id`, shared, author).Scan(&id))
				_, err = f.pool.Exec(ctx, `INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES($1,$2,5),($1,$3,5)`, id, f.user, f.other)
				require.NoError(t, err)
				_, err = f.pool.Exec(ctx, `WITH tag AS (INSERT INTO tags(user_id,name) VALUES($1,$2) RETURNING id) INSERT INTO transaction_tags SELECT $3,id FROM tag`, author, id, id)
				require.NoError(t, err)
			}
			query := `SELECT jsonb_build_array(
 (SELECT to_jsonb(a) FROM accounts a WHERE id=$1),
 (SELECT jsonb_agg(to_jsonb(m) ORDER BY user_id) FROM account_members m WHERE account_id=$1),
 (SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM transactions t WHERE account_id=$1),
 (SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM transaction_shares s WHERE transaction_id IN (SELECT id FROM transactions WHERE account_id=$1)),
 (SELECT jsonb_agg(to_jsonb(l) ORDER BY transaction_id,tag_id) FROM transaction_tags l WHERE transaction_id IN (SELECT id FROM transactions WHERE account_id=$1)))::text`
			var before, after string
			require.NoError(t, f.pool.QueryRow(ctx, query, shared).Scan(&before))
			p := f.replacement(t)
			require.True(t, p.Report.CanImport(), p.Report.Diagnostics)
			if kind == "only_shared" {
				require.Empty(t, p.Replacement.Accounts)
			} else {
				require.Len(t, p.Replacement.Accounts, 1)
				require.Equal(t, a, p.Replacement.Accounts[0].ID)
			}
			for _, account := range p.Accounts {
				if account.SourceID == "cash" {
					require.Equal(t, "Cash (1)", account.Name)
				}
			}
			_, err = f.svc.Confirm(ctx, f.user, p.ID, true, true)
			require.NoError(t, err)
			require.NoError(t, f.pool.QueryRow(ctx, query, shared).Scan(&after))
			require.Equal(t, before, after)
			var exists bool
			require.NoError(t, f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1) OR EXISTS(SELECT 1 FROM transactions WHERE id=$2)`, a, tx).Scan(&exists))
			require.Equal(t, kind == "only_shared", exists)
		})
	}
}
func TestMonefyReplacementDetectsChangesAndRefreshesOptions(t *testing.T) {
	for _, kind := range []string{"amount", "share", "name", "access_mode", "new_account", "deleted_account", "member", "external_link", "tag_link"} {
		t.Run(kind, func(t *testing.T) {
			f := newImportFixture(t)
			ctx := context.Background()
			a, tx := f.oldHistory(t)
			p := f.replacement(t)
			var err error
			switch kind {
			case "amount":
				_, err = f.pool.Exec(ctx, `UPDATE transactions SET amount=43 WHERE id=$1`, tx)
			case "share":
				_, err = f.pool.Exec(ctx, `UPDATE transaction_shares SET amount=43 WHERE transaction_id=$1`, tx)
			case "name":
				_, err = f.pool.Exec(ctx, `UPDATE accounts SET name='Edited' WHERE id=$1`, a)
			case "access_mode":
				_, err = f.pool.Exec(ctx, `UPDATE accounts SET access_mode='shared' WHERE id=$1`, a)
			case "new_account":
				f.oldAccount(t, f.user)
			case "deleted_account":
				_, err = f.pool.Exec(ctx, `UPDATE accounts SET deleted_at=NULL WHERE id=$1`, a)
			case "member":
				_, err = f.pool.Exec(ctx, `UPDATE account_members SET default_share=0.5 WHERE account_id=$1`, a)
			case "external_link":
				other := f.oldAccount(t, f.other)
				_, err = f.pool.Exec(ctx, `UPDATE transactions SET type='transfer',to_account_id=$1 WHERE id=$2`, other, tx)
			case "tag_link":
				_, err = f.pool.Exec(ctx, `WITH tag AS (INSERT INTO tags(user_id,name) VALUES($1,'new') RETURNING id) INSERT INTO transaction_tags SELECT $2,id FROM tag`, f.user, tx)
			}
			require.NoError(t, err)
			_, err = f.svc.Confirm(ctx, f.user, p.ID, true, true)
			require.ErrorContains(t, err, "replacement_changed")
			f.exists(t, a, tx)
			configured, err := f.svc.Configure(ctx, f.user, p.ID, map[string]model.AccountKind{"cash": "spending", "travel": "deposit", "reserve": "investment"}, nil, nil, nil)
			require.NoError(t, err)
			require.NotEqual(t, p.Replacement.Fingerprint, configured.Replacement.Fingerprint)
			if kind == "external_link" {
				require.False(t, configured.Report.CanImport())
				return
			}
			_, err = f.svc.Confirm(ctx, f.user, configured.ID, true, true)
			require.NoError(t, err)
		})
	}
}
func TestMonefyReplacementConflictingWriteRollsBack(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	a, tx := f.oldHistory(t)
	p := f.replacement(t)
	writer, err := f.pool.Begin(ctx)
	require.NoError(t, err)
	defer writer.Rollback(ctx) //nolint:errcheck // Страховочный откат при падении assertion.
	_, err = writer.Exec(ctx, `UPDATE transactions SET amount=43 WHERE id=$1`, tx)
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, true, true)
	require.ErrorIs(t, err, apperr.ErrConflict)
	require.NoError(t, writer.Commit(ctx))
	f.exists(t, a, tx)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, true, true)
	require.ErrorContains(t, err, "replacement_changed")
	// The inverse ordering blocks ordinary edits and insertion of external links.
	locked, err := f.pool.Begin(ctx)
	require.NoError(t, err)
	defer locked.Rollback(ctx) //nolint:errcheck // Страховочный откат при падении assertion.
	require.NoError(t, f.repo.WithTx(locked).LockReplacement(ctx))
	writer, err = f.pool.Begin(ctx)
	require.NoError(t, err)
	defer writer.Rollback(ctx) //nolint:errcheck // Страховочный откат при падении assertion.
	_, err = writer.Exec(ctx, `SET LOCAL lock_timeout='100ms'`)
	require.NoError(t, err)
	_, err = writer.Exec(ctx, `UPDATE transactions SET amount=44 WHERE id=$1`, tx)
	require.Error(t, err)
	require.NoError(t, writer.Rollback(ctx))
	require.NoError(t, locked.Rollback(ctx))
}

func TestMonefyConcurrentDifferentReplacements(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	f.oldHistory(t)
	previews := []model.ImportPreview{f.replacement(t), f.replacement(t)}
	var errs [2]error
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, errs[i] = f.svc.Confirm(ctx, f.user, previews[i].ID, true, true) }()
	}
	wg.Wait()
	success := 0
	for _, err := range errs {
		if err == nil {
			success++
		} else {
			require.ErrorContains(t, err, "replacement_changed")
		}
	}
	require.Equal(t, 1, success)
	var count int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM monefy_imports WHERE user_id=$1`, f.user).Scan(&count))
	require.Equal(t, 1, count)
}
