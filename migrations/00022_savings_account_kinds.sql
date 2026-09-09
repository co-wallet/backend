-- +goose Up
ALTER TABLE accounts DROP CONSTRAINT accounts_kind_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_kind_check
    CHECK (kind IN ('spending', 'savings', 'deposit', 'savings_account', 'investment'));

-- +goose Down
-- Refuse rollback while new kinds exist, preserving their meaning and history.
ALTER TABLE accounts DROP CONSTRAINT accounts_kind_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_kind_check
    CHECK (kind IN ('spending', 'deposit', 'investment'));
