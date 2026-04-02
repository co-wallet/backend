-- +goose Up
ALTER TABLE transactions ADD COLUMN to_amount NUMERIC(15,4);

-- +goose Down
ALTER TABLE transactions DROP COLUMN IF EXISTS to_amount;
