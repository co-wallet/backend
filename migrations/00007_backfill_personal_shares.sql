-- +goose Up
-- Backfill transaction_shares for transactions that have none.
-- For personal accounts (or shared with ≤1 member), the creator owns 100% of the amount.
INSERT INTO transaction_shares (transaction_id, user_id, amount, is_custom)
SELECT t.id, t.created_by, t.amount, false
FROM transactions t
WHERE NOT EXISTS (
    SELECT 1 FROM transaction_shares ts WHERE ts.transaction_id = t.id
);

-- +goose Down
-- Remove only the auto-backfilled rows (is_custom = false, user = created_by, amount = full tx amount).
DELETE FROM transaction_shares ts
USING transactions t
WHERE ts.transaction_id = t.id
  AND ts.user_id = t.created_by
  AND ts.amount = t.amount
  AND ts.is_custom = false;
