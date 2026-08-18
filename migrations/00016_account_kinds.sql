-- +goose Up
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_type_check;
ALTER TABLE accounts RENAME COLUMN type TO access_mode;
ALTER TABLE accounts
    ADD CONSTRAINT accounts_access_mode_check
    CHECK (access_mode IN ('personal', 'shared'));

ALTER TABLE accounts
    ADD COLUMN kind VARCHAR(16) NOT NULL DEFAULT 'spending';
ALTER TABLE accounts
    ADD CONSTRAINT accounts_kind_check
    CHECK (kind IN ('spending', 'deposit', 'investment'));

ALTER TABLE accounts DROP COLUMN include_in_balance;

-- +goose Down
ALTER TABLE accounts
    ADD COLUMN include_in_balance BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_kind_check;
ALTER TABLE accounts DROP COLUMN kind;

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_access_mode_check;
ALTER TABLE accounts RENAME COLUMN access_mode TO type;
ALTER TABLE accounts
    ADD CONSTRAINT accounts_type_check
    CHECK (type IN ('personal', 'shared'));
