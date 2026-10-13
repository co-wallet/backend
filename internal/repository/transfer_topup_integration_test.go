package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/apperr"
	transactionhandler "github.com/co-wallet/backend/internal/handler/transaction"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
	"github.com/stretchr/testify/require"
)

func TestTransferTopupAPI(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	accounts := repository.NewAccountRepository(f.pool)
	svc := service.NewAccountService(f.pool, accounts, repository.NewUserRepository(f.pool))
	source, err := svc.CreateAccount(ctx, f.other, model.CreateAccountReq{Name: "Source", Currency: "USD", AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, AcceptTransfers: true, InitialBalanceDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	destination, err := svc.CreateAccount(ctx, f.user, model.CreateAccountReq{Name: "Destination", Currency: "USD", AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, InitialBalanceDate: source.InitialBalanceDate})
	require.NoError(t, err)
	h, token := f.api(t)
	var choices []map[string]any
	importRequest(t, h, token, "GET", "/api/transfer-accounts?username=other", nil, 200, &choices)
	require.Len(t, choices, 1)
	require.Len(t, choices[0], 4)
	require.Equal(t, source.ID, choices[0]["id"])
	importRequest(t, h, token, "GET", "/api/transfer-accounts", nil, 200, &choices)
	require.Len(t, choices, 1)
	require.Len(t, choices[0], 5)
	require.Equal(t, "other", choices[0]["ownerUsername"])
	importRequest(t, h, "", "GET", "/api/transfer-accounts", nil, 401, nil)
	importRequest(t, h, token, "GET", "/api/accounts/"+source.ID, nil, 403, nil)
	body := []byte(fmt.Sprintf(`{"accountId":%q,"toAccountId":%q,"type":"transfer","amount":12.3456,"currency":"USD","date":"2026-01-01T00:00:00Z"}`, source.ID, destination.ID))
	var created transactionhandler.TransactionResponse
	importRequest(t, h, token, "POST", "/api/transactions", body, 201, &created)
	require.True(t, created.ReadOnly)
	require.Empty(t, created.Shares)
	require.Equal(t, "other", created.AccountOwnerUsername)
	require.Equal(t, "importer", created.ToAccountOwnerUsername)
	importRequest(t, h, token, "PATCH", "/api/transactions/"+created.ID, []byte(`{"amount":20}`), 403, nil)
	importRequest(t, h, token, "DELETE", "/api/transactions/"+created.ID, nil, 403, nil)
	auth := service.NewAuthService(repository.NewUserRepository(f.pool), "synthetic-integration-secret")
	ownerTokens, err := auth.IssueTokens(model.User{ID: f.other})
	require.NoError(t, err)
	var ownerView transactionhandler.TransactionResponse
	importRequest(t, h, ownerTokens.AccessToken, "GET", "/api/transactions/"+created.ID, nil, 200, &ownerView)
	require.False(t, ownerView.ReadOnly)
	require.Equal(t, f.other, ownerView.Shares[0].UserID)
	require.Equal(t, "importer", ownerView.ToAccountOwnerUsername)
	newDestination, err := svc.CreateAccount(ctx, f.user, model.CreateAccountReq{Name: "Second destination", Currency: "EUR", AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, AcceptTransfers: true, InitialBalanceDate: source.InitialBalanceDate})
	require.NoError(t, err)
	changeBody := []byte(fmt.Sprintf(`{"toAccountId":%q,"toAmount":10}`, newDestination.ID))
	var updated transactionhandler.TransactionResponse
	importRequest(t, h, ownerTokens.AccessToken, "PATCH", "/api/transactions/"+created.ID, changeBody, 200, &updated)
	require.Equal(t, newDestination.ID, *updated.ToAccountID)
	require.Equal(t, "importer", updated.ToAccountOwnerUsername)
	require.Equal(t, "EUR", updated.ToCurrency)
	require.Equal(t, 10.0, *updated.ToAmount)
	importRequest(t, h, ownerTokens.AccessToken, "GET", "/api/transactions/"+created.ID, nil, 200, &updated)
	require.Equal(t, "importer", updated.ToAccountOwnerUsername)
	var listed []transactionhandler.TransactionResponse
	importRequest(t, h, ownerTokens.AccessToken, "GET", "/api/transactions", nil, 200, &listed)
	require.Len(t, listed, 1)
	require.Equal(t, "importer", listed[0].ToAccountOwnerUsername)
	_, err = svc.UpdateAccount(ctx, f.user, newDestination.ID, model.UpdateAccountReq{AcceptTransfers: ptr.To(false)})
	require.NoError(t, err)
	importRequest(t, h, ownerTokens.AccessToken, "GET", "/api/transactions/"+created.ID, nil, 200, &updated)
	require.Equal(t, "importer", updated.ToAccountOwnerUsername)
	// A different closed external destination cannot replace the current one.
	importRequest(t, h, ownerTokens.AccessToken, "PATCH", "/api/transactions/"+created.ID, []byte(fmt.Sprintf(`{"toAccountId":%q}`, destination.ID)), 403, nil)
	importRequest(t, h, ownerTokens.AccessToken, "DELETE", "/api/transactions/"+created.ID, nil, 204, nil)
	importRequest(t, h, token, "GET", "/api/transactions/"+created.ID, nil, 404, nil)
	_, err = svc.UpdateAccount(ctx, f.other, source.ID, model.UpdateAccountReq{AcceptTransfers: ptr.To(false)})
	require.NoError(t, err)
	importRequest(t, h, token, "POST", "/api/transactions", body, 403, nil)
}

func TestTransferTopups(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	accounts := repository.NewAccountRepository(f.pool)
	accountService := service.NewAccountService(f.pool, accounts, repository.NewUserRepository(f.pool))
	repo := repository.NewTransactionRepository(f.pool)
	transactions := service.NewTransactionService(f.pool, repo, accounts, repository.NewTagRepository(f.pool))
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	create := func(owner, name, currency string, enabled bool) model.Account {
		a, err := accountService.CreateAccount(ctx, owner, model.CreateAccountReq{Name: name, AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, Currency: currency, AcceptTransfers: enabled, InitialBalanceDate: date})
		require.NoError(t, err)
		return a
	}
	source := create(f.other, "External source", "USD", true)
	hidden := create(f.other, "Hidden", "USD", false)
	deleted := create(f.other, "Deleted", "USD", true)
	require.NoError(t, accounts.SoftDelete(ctx, deleted.ID))
	// Legacy shared accounts with an enabled flag must stay outside the external catalog.
	_, err := accounts.Create(ctx, model.Account{OwnerID: f.other, Name: "Legacy shared", Currency: "USD", AccessMode: model.AccountAccessModeShared, Kind: model.AccountKindSpending, AcceptTransfers: true, InitialBalanceDate: date})
	require.NoError(t, err)
	personal := create(f.user, "Personal", "USD", true)
	shared, err := accountService.CreateAccount(ctx, f.user, model.CreateAccountReq{Name: "Shared", AccessMode: model.AccountAccessModeShared, Kind: model.AccountKindSpending, Currency: "EUR", InitialBalanceDate: date, Members: []model.CreateAccountMemberReq{{Username: "importer", DefaultShare: 0.25}, {Username: "other", DefaultShare: 0.75}}})
	require.NoError(t, err)
	_, err = f.pool.Exec(ctx, `INSERT INTO exchange_rates(base_currency,quote_currency,rate) VALUES('USD','EUR',0.8) ON CONFLICT(base_currency,quote_currency) DO UPDATE SET rate=EXCLUDED.rate`)
	require.NoError(t, err)
	choices, err := accounts.ListTransferAccounts(ctx, "other")
	require.NoError(t, err)
	require.Equal(t, []model.TransferAccount{{ID: source.ID, Name: source.Name, Currency: "USD"}}, choices)
	choices, err = accounts.ListTransferAccounts(ctx, "oth")
	require.NoError(t, err)
	require.Empty(t, choices)
	catalog, err := accounts.ListAvailableTransferAccounts(ctx, f.user)
	require.NoError(t, err)
	require.Equal(t, []model.TransferAccount{{ID: source.ID, Name: source.Name, Currency: "USD", OwnerUsername: "other"}}, catalog)
	for _, destination := range []model.Account{personal, shared} {
		t.Run(destination.Name, func(t *testing.T) {
			req := model.CreateTransactionReq{AccountID: source.ID, ToAccountID: ptr.To(destination.ID), Type: model.TransactionTypeTransfer, Amount: 100.1234, Currency: "USD", Date: date, Description: ptr.To("Topup")}
			credited := req.Amount
			if destination.Currency != source.Currency {
				req.ToAmount = ptr.To(80.4321)
				credited = *req.ToAmount
			}
			created, err := transactions.Create(ctx, f.user, req)
			require.NoError(t, err)
			require.True(t, created.ReadOnly)
			require.Empty(t, created.Shares)
			stored, err := repo.GetByID(ctx, created.ID)
			require.NoError(t, err)
			require.Equal(t, f.user, stored.CreatedBy)
			require.Len(t, stored.Shares, 1)
			require.Equal(t, f.other, stored.Shares[0].UserID)
			require.Equal(t, req.Amount, stored.Shares[0].Amount)
			ownerView, err := transactions.GetByID(ctx, f.other, created.ID)
			require.NoError(t, err)
			require.False(t, ownerView.ReadOnly)
			ownerBalances, err := accounts.ListBalancesByUser(ctx, f.other, "USD")
			require.NoError(t, err)
			require.Equal(t, -req.Amount, ownerBalances[source.ID].BalanceNative)
			balances, err := accounts.ListBalancesByUser(ctx, f.user, "USD")
			require.NoError(t, err)
			share := 1.0
			if destination.AccessMode == model.AccountAccessModeShared {
				share = 0.25
			}
			require.InDelta(t, credited*share, balances[destination.ID].BalanceNative, 1e-10)
			require.Equal(t, credited, balances[destination.ID].TotalNative)
			listed, err := transactions.List(ctx, f.user, model.TransactionFilter{})
			require.NoError(t, err)
			require.Len(t, listed, 1)
			require.True(t, listed[0].ReadOnly)
			require.Empty(t, listed[0].Shares)
			_, err = transactions.Update(ctx, f.user, created.ID, model.UpdateTransactionReq{Amount: ptr.To(200.0)})
			require.ErrorIs(t, err, apperr.ErrForbidden)
			require.ErrorIs(t, transactions.Delete(ctx, f.user, created.ID), apperr.ErrForbidden)
			// Turning availability off blocks new topups without blocking owner deletion.
			_, err = accountService.UpdateAccount(ctx, f.other, source.ID, model.UpdateAccountReq{AcceptTransfers: ptr.To(false)})
			require.NoError(t, err)
			_, err = transactions.Create(ctx, f.user, req)
			require.ErrorIs(t, err, apperr.ErrForbidden)
			require.NoError(t, transactions.Delete(ctx, f.other, created.ID))
			ownerBalances, err = accounts.ListBalancesByUser(ctx, f.other, "USD")
			require.NoError(t, err)
			require.Zero(t, ownerBalances[source.ID].BalanceNative)
			balances, err = accounts.ListBalancesByUser(ctx, f.user, "USD")
			require.NoError(t, err)
			require.Zero(t, balances[destination.ID].BalanceNative)
			_, err = accountService.UpdateAccount(ctx, f.other, source.ID, model.UpdateAccountReq{AcceptTransfers: ptr.To(true)})
			require.NoError(t, err)
		})
	}
	for _, tc := range []struct {
		source, destination string
		want                error
	}{
		{hidden.ID, personal.ID, apperr.ErrForbidden},
		{deleted.ID, personal.ID, apperr.ErrNotFound},
		{source.ID, hidden.ID, apperr.ErrForbidden},
		{source.ID, deleted.ID, apperr.ErrNotFound},
	} {
		_, err := transactions.Create(ctx, f.user, model.CreateTransactionReq{AccountID: tc.source, ToAccountID: ptr.To(tc.destination), Type: model.TransactionTypeTransfer, Amount: 1, Currency: "USD", Date: date})
		require.ErrorIs(t, err, tc.want)
	}
	// A non-transfer cannot use the external account even when availability is enabled.
	_, err = transactions.Create(ctx, f.user, model.CreateTransactionReq{AccountID: source.ID, Type: model.TransactionTypeExpense, Amount: 1, Currency: "USD", Date: date})
	require.ErrorIs(t, err, apperr.ErrForbidden)
	var remaining int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM transaction_shares`).Scan(&remaining))
	require.Zero(t, remaining)
	_, err = f.pool.Exec(ctx, `UPDATE users SET is_active=false WHERE id=$1`, f.other)
	require.NoError(t, err)
	catalog, err = accounts.ListAvailableTransferAccounts(ctx, f.user)
	require.NoError(t, err)
	require.Empty(t, catalog)
}
