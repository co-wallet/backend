BINARY=bin/server
MIGRATIONS_DIR=migrations
MOCKGEN=$(shell go env GOPATH)/bin/mockgen
LOG_FILE=/tmp/cowallet-backend.log

.PHONY: build run restart stop test lint migrate migrate-down migrate-status mock

build:
	go build -o $(BINARY) ./cmd/server

run:
	go run ./cmd/server

restart:
	@lsof -ti :8080 | xargs kill -9 2>/dev/null; true
	@sleep 1
	go run ./cmd/server >> $(LOG_FILE) 2>&1 &
	@sleep 4 && curl -s http://localhost:8080/api/health

stop:
	@lsof -ti :8080 | xargs kill -9 2>/dev/null; true
	@echo "backend stopped"

test:
	go test ./...

lint:
	golangci-lint run ./...

mock:
	$(MOCKGEN) -destination=internal/service/mocks/mock_transaction_repo.go -package=mocks github.com/co-wallet/backend/internal/service TransactionRepo
	$(MOCKGEN) -destination=internal/service/mocks/mock_account_repo_tx.go  -package=mocks github.com/co-wallet/backend/internal/service AccountRepoForTx
	$(MOCKGEN) -destination=internal/service/mocks/mock_category_repo.go    -package=mocks github.com/co-wallet/backend/internal/service CategoryRepo
	$(MOCKGEN) -destination=internal/service/mocks/mock_tag_repo.go         -package=mocks github.com/co-wallet/backend/internal/service TagRepo

migrate:
	GOOSE_DRIVER=pgx GOOSE_DBSTRING=$(DATABASE_URL) goose -dir $(MIGRATIONS_DIR) up

migrate-down:
	GOOSE_DRIVER=pgx GOOSE_DBSTRING=$(DATABASE_URL) goose -dir $(MIGRATIONS_DIR) down

migrate-status:
	GOOSE_DRIVER=pgx GOOSE_DBSTRING=$(DATABASE_URL) goose -dir $(MIGRATIONS_DIR) status
