-- +goose Up
ALTER TABLE accounts ADD COLUMN accept_transfers boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE accounts DROP COLUMN accept_transfers;
