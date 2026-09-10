package repository_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/apperr"
	accounthandler "github.com/co-wallet/backend/internal/handler/account"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountConfigurationAPI(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	h, token := f.api(t)
	repo := repository.NewAccountRepository(f.pool)
	var personal, shared accounthandler.AccountResponse
	importRequest(t, h, token, "POST", "/api/accounts", []byte(`{"name":"Personal","currency":"USD","initialBalanceDate":"2026-01-01"}`), 201, &personal)
	members, err := repo.GetMembers(ctx, personal.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, f.user, members[0].UserID)
	require.Equal(t, 1.0, members[0].DefaultShare)
	importRequest(t, h, token, "POST", "/api/accounts", []byte(`{"name":"Shared","accessMode":"shared","currency":"EUR","initialBalanceDate":"2026-01-01","members":[{"username":"importer","defaultShare":0.3333},{"username":"other","defaultShare":0.6667}]}`), 201, &shared)
	members, err = repo.GetMembers(ctx, shared.ID)
	require.NoError(t, err)
	require.Len(t, members, 2)
	for _, a := range []accounthandler.AccountResponse{personal, shared} {
		for _, request := range []struct {
			method, path, body string
			status             int
		}{
			{"PATCH", "", `{"accessMode":"personal"}`, 400},
			{"PATCH", "", `{"accessMode":"shared"}`, 400},
			{"PATCH", "", `{"members":[]}`, 400},
			{"POST", "/members", `{"username":"other","defaultShare":0.1}`, 409},
			{"PATCH", "/members/" + f.user, `{"defaultShare":0.1}`, 409},
			{"DELETE", "/members/" + f.other, ``, 409},
		} {
			importRequest(t, h, token, request.method, "/api/accounts/"+a.ID+request.path, []byte(request.body), request.status, nil)
		}
		importRequest(t, h, token, "PATCH", "/api/accounts/"+a.ID, []byte(`{"name":"Renamed","icon":"preset:bank","initialBalance":12.3456}`), 200, nil)
		got, err := repo.GetByID(ctx, a.ID)
		require.NoError(t, err)
		require.Equal(t, "Renamed", got.Name)
		require.Equal(t, 12.3456, got.InitialBalance)
		require.Equal(t, a.AccessMode, string(got.AccessMode))
	}
	for _, rawMembers := range []string{
		`[]`,
		`[{"username":"importer","defaultShare":0.99999}]`,
		`[{"username":"importer","defaultShare":0.9}]`,
		`[{"username":"importer","defaultShare":0.5},{"username":"importer","defaultShare":0.5}]`,
		`[{"username":"missing","defaultShare":1}]`,
		`[{"username":"other","defaultShare":1}]`,
	} {
		importRequest(t, h, token, "POST", "/api/accounts", []byte(`{"name":"Invalid","accessMode":"shared","currency":"USD","initialBalanceDate":"2026-01-01","members":`+rawMembers+`}`), 400, nil)
	}
	var count int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM accounts`).Scan(&count))
	require.Equal(t, 2, count)
	// A failure after the account and first membership are inserted rolls everything back.
	_, err = f.pool.Exec(ctx, `ALTER TABLE account_members ADD CONSTRAINT reject_other CHECK(user_id <> '`+f.other+`') NOT VALID`)
	require.NoError(t, err)
	importRequest(t, h, token, "POST", "/api/accounts", []byte(`{"name":"Rollback","accessMode":"shared","currency":"USD","initialBalanceDate":"2026-01-01","members":[{"username":"importer","defaultShare":0.5},{"username":"other","defaultShare":0.5}]}`), 500, nil)
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM accounts`).Scan(&count))
	require.Equal(t, 2, count)
}

func TestFixedSharesPreserveHistoryAndTransferBalances(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	repo := repository.NewAccountRepository(f.pool)
	svc := service.NewAccountService(f.pool, repo, repository.NewUserRepository(f.pool))
	transactions := service.NewTransactionService(f.pool, repository.NewTransactionRepository(f.pool), repo, repository.NewTagRepository(f.pool))
	analytics := repository.NewAnalyticsRepository(f.pool)
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	source, err := svc.CreateAccount(ctx, f.user, model.CreateAccountReq{Name: "Source", AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, Currency: "USD", InitialBalanceDate: date})
	require.NoError(t, err)
	// Seed an existing configuration directly: deployment must preserve it, not normalize history.
	dest, err := repo.Create(ctx, model.Account{OwnerID: f.user, Name: "Existing", AccessMode: model.AccountAccessModeShared, Kind: model.AccountKindSpending, Currency: "EUR", InitialBalance: 12.3456, InitialBalanceDate: date})
	require.NoError(t, err)
	require.NoError(t, repo.AddMember(ctx, model.AccountMember{AccountID: dest.ID, UserID: f.user, DefaultShare: 0.3333}))
	require.NoError(t, repo.AddMember(ctx, model.AccountMember{AccountID: dest.ID, UserID: f.other, DefaultShare: 0.6667}))
	_, err = f.pool.Exec(ctx, `INSERT INTO exchange_rates(base_currency,quote_currency,rate) VALUES('USD','EUR',0.8) ON CONFLICT(base_currency,quote_currency) DO UPDATE SET rate=EXCLUDED.rate`)
	require.NoError(t, err)
	tx, err := transactions.Create(ctx, f.user, model.CreateTransactionReq{AccountID: source.ID, ToAccountID: ptr.To(dest.ID), Type: model.TransactionTypeTransfer, Amount: 10.1234, ToAmount: ptr.To(8.4321), Currency: "USD", Date: date})
	require.NoError(t, err)
	snapshot := func() string {
		var value string
		require.NoError(t, f.pool.QueryRow(ctx, `SELECT json_build_object('accounts',(SELECT json_agg(a ORDER BY id) FROM accounts a),'members',(SELECT json_agg(m ORDER BY account_id,user_id) FROM account_members m),'transactions',(SELECT json_agg(t ORDER BY id) FROM transactions t),'shares',(SELECT json_agg(s ORDER BY id) FROM transaction_shares s))::text`).Scan(&value))
		return value
	}
	before := snapshot()
	_, err = svc.UpdateAccount(ctx, f.user, dest.ID, model.UpdateAccountReq{AccessMode: ptr.To(model.AccountAccessModePersonal)})
	require.ErrorIs(t, err, apperr.ErrConflict)
	_, err = svc.AddMember(ctx, dest.ID, "other", 0.5)
	require.ErrorIs(t, err, apperr.ErrConflict)
	_, err = svc.UpdateMember(ctx, dest.ID, f.user, 0.5)
	require.ErrorIs(t, err, apperr.ErrConflict)
	require.ErrorIs(t, svc.RemoveMember(ctx, f.user, dest.ID, f.other), apperr.ErrConflict)
	require.JSONEq(t, before, snapshot())
	check := func(received float64) {
		var sum float64
		for _, member := range []struct {
			id    string
			share float64
		}{{f.user, 0.3333}, {f.other, 0.6667}} {
			balances, err := repo.ListBalancesByUser(ctx, member.id, "USD")
			require.NoError(t, err)
			b := balances[dest.ID]
			require.InDelta(t, (12.3456+received)*member.share, b.BalanceNative, 1e-10)
			require.InDelta(t, b.BalanceNative/0.8, b.BalanceDisplay, 1e-10)
			require.InDelta(t, 12.3456+received, b.TotalNative, 1e-10)
			sum += b.BalanceNative
			summary, err := analytics.Summary(ctx, model.AnalyticsFilter{UserID: member.id, AccountIDs: []string{dest.ID}, DisplayCurrency: "USD", DateFrom: date, DateTo: date, IncludeTransferIncome: true})
			require.NoError(t, err)
			require.InDelta(t, b.BalanceDisplay, summary.Balance, 1e-10)
			require.InDelta(t, received*member.share/0.8, summary.Income, 1e-10)
		}
		require.InDelta(t, 12.3456+received, sum, 1e-10)
	}
	check(8.4321)
	_, err = transactions.Update(ctx, f.user, tx.ID, model.UpdateTransactionReq{Amount: ptr.To(20.2345), ToAmount: ptr.To(16.7891)})
	require.NoError(t, err)
	check(16.7891)
	require.NoError(t, transactions.Delete(ctx, f.user, tx.ID))
	check(0)
	// Same-currency fallback and amounts smaller than one displayed cent remain exact in SQL.
	same, err := svc.CreateAccount(ctx, f.user, model.CreateAccountReq{Name: "Same", AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, Currency: "EUR", InitialBalanceDate: date})
	require.NoError(t, err)
	_, err = transactions.Create(ctx, f.user, model.CreateTransactionReq{AccountID: same.ID, ToAccountID: ptr.To(dest.ID), Type: model.TransactionTypeTransfer, Amount: 0.0001, Currency: "EUR", Date: date})
	require.NoError(t, err)
	check(0.0001)
	// Custom expense shares still override the fixed defaults.
	custom, err := transactions.Create(ctx, f.user, model.CreateTransactionReq{AccountID: dest.ID, Type: model.TransactionTypeExpense, Amount: 1, Currency: "EUR", Date: date, Shares: []model.ShareReq{{UserID: f.user, Amount: 0.9}, {UserID: f.other, Amount: 0.1}}})
	require.NoError(t, err)
	encoded, err := json.Marshal(custom.Shares)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `0.9`)
}
