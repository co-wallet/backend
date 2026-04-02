-- +goose Up
ALTER TABLE transactions
    ADD COLUMN default_currency        VARCHAR(3),
    ADD COLUMN default_currency_amount NUMERIC(15,4);

-- +goose Down
ALTER TABLE transactions
    DROP COLUMN default_currency_amount,
    DROP COLUMN default_currency;
