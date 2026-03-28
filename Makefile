BINARY=bin/server
MIGRATIONS_DIR=migrations

.PHONY: build run test lint migrate

build:
	go build -o $(BINARY) ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...

lint:
	golangci-lint run ./...

migrate:
	GOOSE_DRIVER=pgx GOOSE_DBSTRING=$(DATABASE_URL) goose -dir $(MIGRATIONS_DIR) up

migrate-down:
	GOOSE_DRIVER=pgx GOOSE_DBSTRING=$(DATABASE_URL) goose -dir $(MIGRATIONS_DIR) down

migrate-status:
	GOOSE_DRIVER=pgx GOOSE_DBSTRING=$(DATABASE_URL) goose -dir $(MIGRATIONS_DIR) status
