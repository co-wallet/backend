-- +goose Up
-- Only the completion receipt is durable; preview contents live in temporary files.
CREATE TABLE monefy_imports (
    preview_id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    accounts INTEGER NOT NULL,
    categories INTEGER NOT NULL,
    reused_categories INTEGER NOT NULL,
    transactions INTEGER NOT NULL,
    transfers INTEGER NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX monefy_imports_user_id ON monefy_imports(user_id);

-- +goose Down
DROP TABLE monefy_imports;
