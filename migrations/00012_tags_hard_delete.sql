-- +goose Up
-- Switch tags to hard-delete: add CASCADE on transaction_tags FK and drop deleted_at
ALTER TABLE transaction_tags DROP CONSTRAINT transaction_tags_tag_id_fkey;
ALTER TABLE transaction_tags ADD CONSTRAINT transaction_tags_tag_id_fkey
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE;
ALTER TABLE tags DROP COLUMN deleted_at;

-- +goose Down
ALTER TABLE tags ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE transaction_tags DROP CONSTRAINT transaction_tags_tag_id_fkey;
ALTER TABLE transaction_tags ADD CONSTRAINT transaction_tags_tag_id_fkey
    FOREIGN KEY (tag_id) REFERENCES tags(id);
