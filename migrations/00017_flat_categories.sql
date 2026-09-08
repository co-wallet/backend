-- +goose Up
-- Also upgrade databases created with earlier versions of migrations 3 and 4.
DROP INDEX IF EXISTS idx_categories_unique_name;
ALTER TABLE categories DROP COLUMN IF EXISTS parent_id;

-- Keep every category ID and transaction link. Resolve active name collisions
-- deterministically, reserving existing names before adding numeric suffixes.
-- +goose StatementBegin
DO $$
DECLARE
    category RECORD;
    candidate TEXT;
    suffix TEXT;
    counter INTEGER;
BEGIN
    FOR category IN
        SELECT id, user_id, type, name FROM (
            SELECT id, user_id, type, name,
                   row_number() OVER (
                       PARTITION BY user_id, type, lower(name)
                       ORDER BY created_at, id
                   ) AS position
            FROM categories WHERE deleted_at IS NULL
        ) ranked
        WHERE position > 1 ORDER BY id
    LOOP
        counter := 2;
        LOOP
            suffix := ' (' || counter || ')';
            candidate := left(category.name, 100 - length(suffix)) || suffix;
            EXIT WHEN NOT EXISTS (
                SELECT 1 FROM categories
                WHERE user_id = category.user_id AND type = category.type
                  AND deleted_at IS NULL AND lower(name) = lower(candidate)
            );
            counter := counter + 1;
        END LOOP;
        UPDATE categories SET name = candidate WHERE id = category.id;
    END LOOP;
END $$;
-- +goose StatementEnd

CREATE UNIQUE INDEX idx_categories_unique_name
    ON categories (user_id, type, lower(name))
    WHERE deleted_at IS NULL;

-- +goose Down
-- Irreversible data migration: earlier schema migrations already define a flat
-- list. Category names and transaction references remain valid on rollback.
SELECT 1;
