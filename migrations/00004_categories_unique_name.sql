-- +goose Up
-- Active category names are unique per user and type, ignoring case.
CREATE UNIQUE INDEX idx_categories_unique_name
    ON categories (user_id, type, lower(name))
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX idx_categories_unique_name;
