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
	$(MOCKGEN) -source=internal/service/account.go  -destination=internal/service/mocks/mock_account_repo.go  -package=mocks
	$(MOCKGEN) -source=internal/service/admin.go    -destination=internal/service/mocks/mock_admin_repo.go    -package=mocks
	$(MOCKGEN) -source=internal/service/currency.go -destination=internal/service/mocks/mock_currency_repo.go -package=mocks
	$(MOCKGEN) -source=internal/service/invite.go   -destination=internal/service/mocks/mock_invite_repo.go   -package=mocks
	$(MOCKGEN) -source=internal/middleware/auth.go    -destination=internal/middleware/mocks/mock_token_validator.go -package=mocks
	$(MOCKGEN) -source=internal/middleware/account.go -destination=internal/middleware/mocks/mock_member_checker.go  -package=mocks
	$(MOCKGEN) -source=internal/handler/auth/handler.go        -destination=internal/handler/auth/mocks/mock_auth_service.go               -package=mocks
	$(MOCKGEN) -source=internal/handler/transaction/handler.go -destination=internal/handler/transaction/mocks/mock_transaction_service.go -package=mocks
	$(MOCKGEN) -source=internal/handler/account/handler.go     -destination=internal/handler/account/mocks/mock_account_service.go         -package=mocks
	$(MOCKGEN) -source=internal/handler/category/handler.go    -destination=internal/handler/category/mocks/mock_category_service.go       -package=mocks
	$(MOCKGEN) -source=internal/handler/tag/handler.go         -destination=internal/handler/tag/mocks/mock_tag_service.go                 -package=mocks
	$(MOCKGEN) -source=internal/handler/invite/handler.go      -destination=internal/handler/invite/mocks/mock_invite_service.go           -package=mocks
	$(MOCKGEN) -source=internal/handler/analytics/handler.go   -destination=internal/handler/analytics/mocks/mock_analytics_service.go     -package=mocks
	$(MOCKGEN) -source=internal/handler/admin/handler.go       -destination=internal/handler/admin/mocks/mock_admin_service.go             -package=mocks
	$(MOCKGEN) -source=internal/handler/currency/handler.go    -destination=internal/handler/currency/mocks/mock_currency_service.go       -package=mocks

migrate:
	GOOSE_DRIVER=pgx GOOSE_DBSTRING=$(DATABASE_URL) goose -dir $(MIGRATIONS_DIR) up

migrate-down:
	GOOSE_DRIVER=pgx GOOSE_DBSTRING=$(DATABASE_URL) goose -dir $(MIGRATIONS_DIR) down

migrate-status:
	GOOSE_DRIVER=pgx GOOSE_DBSTRING=$(DATABASE_URL) goose -dir $(MIGRATIONS_DIR) status
