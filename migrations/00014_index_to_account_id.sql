-- +goose Up
CREATE INDEX idx_transactions_to_account_id ON transactions(to_account_id) WHERE to_account_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_transactions_to_account_id;
