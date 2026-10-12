-- +goose Up
CREATE TABLE account_order (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (user_id, account_id)
);

-- +goose Down
DROP TABLE account_order;
