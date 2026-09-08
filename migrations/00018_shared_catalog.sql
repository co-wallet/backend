-- +goose Up
-- Preserve history while merging per-user catalog duplicates into shared entries.
DROP INDEX idx_categories_unique_name;
CREATE TEMP TABLE category_merge ON COMMIT DROP AS
SELECT id, first_value(id) OVER (PARTITION BY type, lower(name) ORDER BY created_at, id) AS canonical_id
FROM categories;
CREATE TABLE hidden_categories (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, category_id)
);
INSERT INTO hidden_categories (user_id, category_id)
SELECT DISTINCT c.user_id, m.canonical_id FROM categories c JOIN category_merge m USING (id)
WHERE c.deleted_at IS NOT NULL;
UPDATE transactions t SET category_id = m.canonical_id FROM category_merge m
WHERE t.category_id = m.id AND m.id <> m.canonical_id;
DELETE FROM categories c USING category_merge m WHERE c.id = m.id AND m.id <> m.canonical_id;
ALTER TABLE categories DROP COLUMN deleted_at;
CREATE UNIQUE INDEX idx_categories_unique_name ON categories (type, lower(name));

ALTER TABLE tags DROP CONSTRAINT tags_user_id_name_key;
CREATE TEMP TABLE tag_merge ON COMMIT DROP AS
SELECT id, first_value(id) OVER (PARTITION BY lower(name) ORDER BY created_at, id) AS canonical_id
FROM tags;
INSERT INTO transaction_tags (transaction_id, tag_id)
SELECT tt.transaction_id, m.canonical_id FROM transaction_tags tt JOIN tag_merge m ON m.id = tt.tag_id
ON CONFLICT DO NOTHING;
DELETE FROM transaction_tags tt USING tag_merge m WHERE tt.tag_id = m.id AND m.id <> m.canonical_id;
DELETE FROM tags t USING tag_merge m WHERE t.id = m.id AND m.id <> m.canonical_id;
UPDATE tags SET name = lower(name);
CREATE UNIQUE INDEX idx_tags_unique_name ON tags (lower(name));
CREATE TABLE hidden_tags (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, tag_id)
);
-- Never remove historical tag links when a catalog entry is deleted.
ALTER TABLE transaction_tags DROP CONSTRAINT transaction_tags_tag_id_fkey;
ALTER TABLE transaction_tags ADD CONSTRAINT transaction_tags_tag_id_fkey
    FOREIGN KEY (tag_id) REFERENCES tags(id);
-- user_id in catalog tables records the creator, not ownership or visibility.

-- +goose Down
-- Merged entries cannot be separated back into their original per-user records.
DROP TABLE hidden_tags;
DROP TABLE hidden_categories;
DROP INDEX idx_tags_unique_name;
ALTER TABLE tags ADD CONSTRAINT tags_user_id_name_key UNIQUE (user_id, name);
ALTER TABLE categories ADD COLUMN deleted_at TIMESTAMPTZ;
DROP INDEX idx_categories_unique_name;
CREATE UNIQUE INDEX idx_categories_unique_name ON categories (user_id, type, lower(name)) WHERE deleted_at IS NULL;
ALTER TABLE transaction_tags DROP CONSTRAINT transaction_tags_tag_id_fkey;
ALTER TABLE transaction_tags ADD CONSTRAINT transaction_tags_tag_id_fkey
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE;
