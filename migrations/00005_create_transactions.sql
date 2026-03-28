-- +goose Up
CREATE TABLE transactions (
    id               UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id       UUID          NOT NULL REFERENCES accounts(id),
    to_account_id    UUID          REFERENCES accounts(id),
    type             VARCHAR(10)   NOT NULL CHECK (type IN ('expense', 'income', 'transfer')),
    amount           NUMERIC(15,2) NOT NULL CHECK (amount > 0),
    currency         VARCHAR(3)    NOT NULL,
    exchange_rate    NUMERIC(15,6),
    category_id      UUID          REFERENCES categories(id),
    description      TEXT,
    date             DATE          NOT NULL,
    include_in_balance BOOLEAN     NOT NULL DEFAULT true,
    created_by       UUID          NOT NULL REFERENCES users(id),
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE TABLE transaction_shares (
    id             UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID          NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    user_id        UUID          NOT NULL REFERENCES users(id),
    amount         NUMERIC(15,2) NOT NULL,
    is_custom      BOOLEAN       NOT NULL DEFAULT false,
    UNIQUE (transaction_id, user_id)
);

CREATE INDEX idx_transactions_account_id  ON transactions(account_id);
CREATE INDEX idx_transactions_date        ON transactions(date DESC);
CREATE INDEX idx_transactions_created_by  ON transactions(created_by);
CREATE INDEX idx_transaction_shares_tx_id ON transaction_shares(transaction_id);

-- +goose Down
DROP TABLE transaction_shares;
DROP TABLE transactions;
