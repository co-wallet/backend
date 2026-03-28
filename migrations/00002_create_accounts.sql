-- +goose Up
CREATE TABLE accounts (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id             UUID         NOT NULL REFERENCES users(id),
    name                 VARCHAR(100) NOT NULL,
    type                 VARCHAR(10)  NOT NULL CHECK (type IN ('personal', 'shared')),
    currency             VARCHAR(3)   NOT NULL,
    icon                 VARCHAR(50),
    include_in_balance   BOOLEAN      NOT NULL DEFAULT true,
    initial_balance      NUMERIC(15,2) NOT NULL DEFAULT 0,
    initial_balance_date DATE,
    deleted_at           TIMESTAMPTZ,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE account_members (
    account_id    UUID          NOT NULL REFERENCES accounts(id),
    user_id       UUID          NOT NULL REFERENCES users(id),
    default_share NUMERIC(5,4)  NOT NULL CHECK (default_share >= 0 AND default_share <= 1),
    PRIMARY KEY (account_id, user_id)
);

CREATE INDEX idx_accounts_owner_id ON accounts(owner_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_account_members_user_id ON account_members(user_id);

-- +goose Down
DROP INDEX idx_account_members_user_id;
DROP INDEX idx_accounts_owner_id;
DROP TABLE account_members;
DROP TABLE accounts;
