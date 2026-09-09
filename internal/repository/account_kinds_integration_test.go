package repository_test

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	accounthandler "github.com/co-wallet/backend/internal/handler/account"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestAccountKindsCreationBalancesAndFilters(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	h, token := f.api(t)
	accounts := repository.NewAccountRepository(f.pool)
	analytics := repository.NewAnalyticsRepository(f.pool)
	txs := repository.NewTransactionRepository(f.pool)
	kinds := []model.AccountKind{model.AccountKindSpending, model.AccountKindSavings, model.AccountKindDeposit, model.AccountKindSavingsAccount, model.AccountKindInvestment}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"name": string(kind), "kind": kind, "accessMode": "shared", "currency": "USD", "initialBalance": 100, "initialBalanceDate": time.Now().Format("2006-01-02")})
			require.NoError(t, err)
			var created accounthandler.AccountResponse
			importRequest(t, h, token, "POST", "/api/accounts", body, 201, &created)
			require.Equal(t, string(kind), created.Kind)
			_, err = f.pool.Exec(ctx, `UPDATE account_members SET default_share=0.25 WHERE account_id=$1`, created.ID)
			require.NoError(t, err)
			_, err = f.pool.Exec(ctx, `INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,0.75)`, created.ID, f.other)
			require.NoError(t, err)
			tx, err := txs.Create(ctx, model.Transaction{AccountID: created.ID, Type: model.TransactionTypeExpense, Amount: 40, Currency: "USD", Date: time.Now(), CreatedBy: f.user, Shares: []model.TransactionShare{{UserID: f.user, Amount: 5, IsCustom: true}, {UserID: f.other, Amount: 35, IsCustom: true}}})
			require.NoError(t, err)
			balances, err := accounts.ListBalancesByUser(ctx, f.user, "USD")
			require.NoError(t, err)
			require.Equal(t, 20.0, balances[created.ID].BalanceNative)
			require.Equal(t, 60.0, balances[created.ID].TotalNative)
			summary, err := analytics.Summary(ctx, model.AnalyticsFilter{UserID: f.user, DisplayCurrency: "USD", AccountKinds: []model.AccountKind{kind}, DateFrom: time.Now().AddDate(0, 0, -1), DateTo: time.Now().AddDate(0, 0, 1)})
			require.NoError(t, err)
			require.Equal(t, model.AnalyticsSummary{Balance: 20, Expenses: 5}, summary)
			listed, err := txs.List(ctx, f.user, model.TransactionFilter{AccountIDs: []string{created.ID}})
			require.NoError(t, err)
			require.Len(t, listed, 1)
			require.Equal(t, tx.ID, listed[0].ID)
			require.Equal(t, kind, listed[0].Account.Kind)
		})
	}
	summary, err := analytics.Summary(ctx, model.AnalyticsFilter{UserID: f.user, DisplayCurrency: "USD", AccountKinds: []model.AccountKind{model.AccountKindSavings, model.AccountKindSavingsAccount}, DateFrom: time.Now().AddDate(0, 0, -1), DateTo: time.Now().AddDate(0, 0, 1)})
	require.NoError(t, err)
	require.Equal(t, model.AnalyticsSummary{Balance: 40, Expenses: 10}, summary)
	var response map[string]string
	importRequest(t, h, token, "POST", "/api/accounts", []byte(`{"name":"Invalid","kind":"unknown","currency":"USD","initialBalanceDate":"2026-01-01"}`), 400, &response)
	require.Contains(t, response["error"], "savings_account")
	// Omitting kind remains compatible with older clients.
	var legacy accounthandler.AccountResponse
	importRequest(t, h, token, "POST", "/api/accounts", []byte(`{"name":"Legacy","currency":"USD","initialBalanceDate":"2026-01-01"}`), 201, &legacy)
	require.Equal(t, "spending", legacy.Kind)
}

func TestMonefyAPISavingsKinds(t *testing.T) {
	f := newImportFixture(t)
	h, token := f.api(t)
	source, err := os.ReadFile(f.source)
	require.NoError(t, err)
	var p apiPreview
	importRequest(t, h, token, "POST", "/api/imports/monefy/preview", source, 201, &p)
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/options", []byte(`{"account_kinds":{"cash":"unknown","travel":"savings_account","reserve":"investment"}}`), 400, nil)
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/options", []byte(`{"account_kinds":{"cash":"savings","travel":"savings_account","reserve":"investment"}}`), 201, &p)
	require.True(t, p.CanConfirm)
	expected := map[string]string{"cash": "savings", "travel": "savings_account", "reserve": "investment"}
	for _, a := range p.Accounts {
		require.Equal(t, expected[a.ID], a.Kind)
	}
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/confirm", []byte(`{"acknowledge_exclusions":true}`), 200, nil)
	var imported []accounthandler.AccountResponse
	importRequest(t, h, token, "GET", "/api/accounts?currency=RUB", nil, 200, &imported)
	require.Len(t, imported, 3)
	for _, a := range imported {
		matched := false
		for _, preview := range p.Accounts {
			if preview.Name == a.Name {
				require.Equal(t, preview.Kind, a.Kind)
				expectedBalance, err := strconv.ParseFloat(preview.Balance, 64)
				require.NoError(t, err)
				require.NotNil(t, a.Balance)
				require.InDelta(t, expectedBalance, a.Balance.Native, 0.0001)
				require.InDelta(t, expectedBalance, a.Balance.TotalNative, 0.0001)
				matched = true
			}
		}
		require.True(t, matched, "imported account must match preview")
	}
}

func TestAccountKindsMigrationPreservesHistory(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	d := stdlib.OpenDB(*f.pool.Config().ConnConfig)
	defer d.Close() //nolint:errcheck // Закрываем тестовое подключение.
	require.NoError(t, goose.DownTo(d, "../../migrations", 21))
	repo := repository.NewAccountRepository(f.pool)
	for _, kind := range []model.AccountKind{model.AccountKindSpending, model.AccountKindDeposit, model.AccountKindInvestment} {
		a, err := repo.Create(ctx, model.Account{OwnerID: f.user, Name: string(kind), Kind: kind, AccessMode: model.AccountAccessModePersonal, Currency: "USD", InitialBalance: 123.4567, InitialBalanceDate: time.Now()})
		require.NoError(t, err)
		_, err = repository.NewTransactionRepository(f.pool).Create(ctx, model.Transaction{AccountID: a.ID, Type: model.TransactionTypeIncome, Amount: 12.3456, Currency: "USD", Date: time.Now(), CreatedBy: f.user, Shares: []model.TransactionShare{{UserID: f.user, Amount: 12.3456}}})
		require.NoError(t, err)
	}
	before, err := repo.ListByUser(ctx, f.user)
	require.NoError(t, err)
	// Owners are listed even without membership; inspect all stored records as well.
	var historyBefore, historyAfter string
	snapshot := `SELECT json_build_object('accounts',(SELECT json_agg(a ORDER BY id) FROM accounts a),'transactions',(SELECT json_agg(t ORDER BY id) FROM transactions t),'shares',(SELECT json_agg(s ORDER BY id) FROM transaction_shares s))::text`
	require.NoError(t, f.pool.QueryRow(ctx, snapshot).Scan(&historyBefore))
	require.NoError(t, goose.Up(d, "../../migrations"))
	after, err := repo.ListByUser(ctx, f.user)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, f.pool.QueryRow(ctx, snapshot).Scan(&historyAfter))
	require.Equal(t, historyBefore, historyAfter)
	require.NoError(t, goose.DownTo(d, "../../migrations", 21))
	require.NoError(t, goose.Up(d, "../../migrations"))
	_, err = repo.Create(ctx, model.Account{OwnerID: f.user, Name: "New", Kind: model.AccountKindSavingsAccount, AccessMode: model.AccountAccessModePersonal, Currency: "USD", InitialBalanceDate: time.Now()})
	require.NoError(t, err)
	// A rollback cannot silently reclassify newly created accounts.
	require.Error(t, goose.DownTo(d, "../../migrations", 21))
	_, err = repo.Create(ctx, model.Account{OwnerID: f.user, Name: "Still supported", Kind: model.AccountKindSavings, AccessMode: model.AccountAccessModePersonal, Currency: "USD", InitialBalanceDate: time.Now()})
	require.NoError(t, err)
	_, err = repo.Create(ctx, model.Account{OwnerID: f.user, Name: "Invalid", Kind: "unknown", AccessMode: model.AccountAccessModePersonal, Currency: "USD", InitialBalanceDate: time.Now()})
	require.Error(t, err)
}
