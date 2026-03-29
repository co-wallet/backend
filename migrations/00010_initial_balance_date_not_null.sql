-- +goose Up
-- Backfill: set initial_balance_date = created_at where NULL, initial_balance = 0 where NULL
UPDATE accounts
SET initial_balance_date = created_at::date
WHERE initial_balance_date IS NULL;

ALTER TABLE accounts
    ALTER COLUMN initial_balance_date SET NOT NULL;

-- +goose Down
ALTER TABLE accounts
    ALTER COLUMN initial_balance_date DROP NOT NULL;
