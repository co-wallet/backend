-- +goose Up
ALTER TABLE transactions DROP COLUMN include_in_balance;

-- +goose Down
ALTER TABLE transactions ADD COLUMN include_in_balance BOOLEAN NOT NULL DEFAULT true;
