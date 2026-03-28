-- +goose Up
-- Unique name within the same parent scope per user (only among non-deleted rows).
-- COALESCE maps NULL parent_id to a fixed sentinel so root-level categories
-- are also covered by the unique index.
CREATE UNIQUE INDEX idx_categories_unique_name
    ON categories (user_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name))
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX idx_categories_unique_name;
