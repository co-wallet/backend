package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
)

// These short-lived table locks also cover updates/deletes and new external
// links, which the user FK lock alone cannot protect. NOWAIT avoids lock-order
// deadlocks with ordinary writes that already hold a table lock and need the
// user FK. Contention fails the entire import without deleting anything.
func (r *ImportRepository) LockReplacement(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `LOCK TABLE accounts, account_members, transactions,
 transaction_shares, transaction_tags IN SHARE ROW EXCLUSIVE MODE NOWAIT`)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
		return apperr.ErrConflict
	}
	return err
}

// One statement gives the preview a consistent view of all related rows.
// Only the SHA-256 and the account/count manifest are persisted, not old amounts
// or descriptions. Include full row contents so same-count edits invalidate it.
func (r *ImportRepository) Replacement(ctx context.Context, user string) (model.ImportReplacement, error) {
	var accountsJSON, countsJSON, blockersJSON []byte
	var contents string
	err := r.db.QueryRow(ctx, `WITH
 owned AS (SELECT * FROM accounts WHERE owner_id=$1),
 related AS (SELECT t.* FROM transactions t WHERE
   t.account_id IN (SELECT id FROM owned) OR t.to_account_id IN (SELECT id FROM owned)
   OR t.created_by=$1 OR EXISTS(SELECT 1 FROM transaction_shares s WHERE s.transaction_id=t.id AND s.user_id=$1)),
 members AS (SELECT * FROM account_members WHERE account_id IN (SELECT id FROM owned) OR user_id=$1),
 shares AS (SELECT * FROM transaction_shares WHERE transaction_id IN (SELECT id FROM related)),
 links AS (SELECT * FROM transaction_tags WHERE transaction_id IN (SELECT id FROM related))
 SELECT
 COALESCE((SELECT jsonb_agg(jsonb_build_object('ID',id,'Name',name,'Currency',currency,'DeletedAt',deleted_at) ORDER BY id) FROM owned),'[]'),
 jsonb_build_object(
   'accounts',(SELECT count(*) FROM owned),
   'deleted_accounts',(SELECT count(*) FROM owned WHERE deleted_at IS NOT NULL),
   'transactions',(SELECT count(*) FROM related WHERE type<>'transfer'),
   'transfers',(SELECT count(*) FROM related WHERE type='transfer'),
   'members',(SELECT count(*) FROM members),
   'shares',(SELECT count(*) FROM shares),
   'tag_links',(SELECT count(*) FROM links)),
 jsonb_build_object(
   'shared_accounts',(SELECT count(*) FROM owned WHERE access_mode<>'personal'),
   'foreign_membership',(SELECT count(*) FROM members WHERE account_id NOT IN (SELECT id FROM owned)),
   'foreign_members',(SELECT count(*) FROM members WHERE user_id<>$1),
   'external_transactions',(SELECT count(*) FROM related WHERE account_id NOT IN (SELECT id FROM owned) OR (to_account_id IS NOT NULL AND to_account_id NOT IN (SELECT id FROM owned))),
   'foreign_authors',(SELECT count(*) FROM related WHERE created_by<>$1),
   'foreign_shares',(SELECT count(*) FROM shares WHERE user_id<>$1)),
 jsonb_build_array(
   (SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM owned o),
   (SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM related t),
   (SELECT jsonb_agg(to_jsonb(m) ORDER BY account_id,user_id) FROM members m),
   (SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM shares s),
   (SELECT jsonb_agg(to_jsonb(l) ORDER BY transaction_id,tag_id) FROM links l))::text`, user).Scan(&accountsJSON, &countsJSON, &blockersJSON, &contents)
	out := model.ImportReplacement{}
	if err != nil {
		return out, err
	}
	hash := sha256.Sum256([]byte(contents))
	out.Fingerprint = hex.EncodeToString(hash[:])
	if err = json.Unmarshal(accountsJSON, &out.Accounts); err != nil {
		return out, err
	}
	if err = json.Unmarshal(countsJSON, &out.Counts); err != nil {
		return out, err
	}
	err = json.Unmarshal(blockersJSON, &out.Blockers)
	return out, err
}

// Caller must hold LockReplacement, validate the unchanged manifest and block
// all external links before invoking this method. Profile and shared catalog
// (including visibility preferences) are intentionally untouched.
func (r *ImportRepository) DeleteReplacement(ctx context.Context, user string) error {
	for _, query := range []string{
		`DELETE FROM transactions WHERE account_id IN (SELECT id FROM accounts WHERE owner_id=$1)`,
		`DELETE FROM account_members WHERE account_id IN (SELECT id FROM accounts WHERE owner_id=$1)`,
		`DELETE FROM accounts WHERE owner_id=$1`,
	} {
		if _, err := r.db.Exec(ctx, query, user); err != nil {
			return err
		}
	}
	return nil
}
