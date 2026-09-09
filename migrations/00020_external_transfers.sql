-- +goose Up
ALTER TABLE accounts ADD COLUMN accept_transfers boolean NOT NULL DEFAULT false;
CREATE TABLE transfer_shares (
 transaction_id uuid NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
 user_id uuid NOT NULL REFERENCES users(id),
 amount numeric(20, 4) NOT NULL,
 PRIMARY KEY (transaction_id, user_id)
);
-- Preserve the distribution in effect at migration for existing incoming transfers.
INSERT INTO transfer_shares (transaction_id, user_id, amount)
SELECT t.id, COALESCE(am.user_id, a.owner_id),
 COALESCE(t.to_amount, t.amount) * COALESCE(am.default_share, 1)
FROM transactions t JOIN accounts a ON a.id = t.to_account_id
LEFT JOIN account_members am ON am.account_id = a.id
WHERE t.type = 'transfer';
-- +goose Down
DROP TABLE transfer_shares;
ALTER TABLE accounts DROP COLUMN accept_transfers;
