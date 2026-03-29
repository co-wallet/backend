-- +goose Up
ALTER TABLE transactions      ALTER COLUMN amount          TYPE NUMERIC(15,4);
ALTER TABLE transaction_shares ALTER COLUMN amount         TYPE NUMERIC(15,4);
ALTER TABLE accounts          ALTER COLUMN initial_balance TYPE NUMERIC(15,4);

-- +goose Down
ALTER TABLE transactions      ALTER COLUMN amount          TYPE NUMERIC(15,2);
ALTER TABLE transaction_shares ALTER COLUMN amount         TYPE NUMERIC(15,2);
ALTER TABLE accounts          ALTER COLUMN initial_balance TYPE NUMERIC(15,2);
