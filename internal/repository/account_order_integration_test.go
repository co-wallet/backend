package repository_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	accounthandler "github.com/co-wallet/backend/internal/handler/account"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAccountOrderAPI(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	h, token := f.api(t)
	repo := repository.NewAccountRepository(f.pool)
	svc := service.NewAccountService(f.pool, repo, repository.NewUserRepository(f.pool))
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	create := func(name string, shared bool) model.Account {
		req := model.CreateAccountReq{Name: name, AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, Currency: "USD", InitialBalanceDate: date}
		if shared {
			req.AccessMode = model.AccountAccessModeShared
			req.Members = []model.CreateAccountMemberReq{{Username: "importer", DefaultShare: 0.5}, {Username: "other", DefaultShare: 0.5}}
		}
		a, err := svc.CreateAccount(ctx, f.user, req)
		require.NoError(t, err)
		return a
	}
	first := create("First", true)
	second := create("Second", true)
	personal := create("Personal", false)
	list := func() []string {
		var result []accounthandler.AccountResponse
		importRequest(t, h, token, "GET", "/api/accounts?currency=USD", nil, 200, &result)
		ids := make([]string, len(result))
		for i, a := range result {
			ids[i] = a.ID
		}
		return ids
	}
	save := func(ids []string, status int) {
		body, err := json.Marshal(map[string]any{"accountIds": ids})
		require.NoError(t, err)
		importRequest(t, h, token, "PATCH", "/api/accounts/order", body, status, nil)
	}
	initial := []string{first.ID, second.ID, personal.ID}
	require.Equal(t, initial, list())
	ordered := []string{personal.ID, second.ID, first.ID}
	save(ordered, 204)
	require.Equal(t, ordered, list())
	// Re-reading through a fresh service verifies persistence rather than in-memory ordering.
	fresh := service.NewAccountService(f.pool, repository.NewAccountRepository(f.pool), repository.NewUserRepository(f.pool))
	otherAccounts, err := fresh.ListByUser(ctx, f.other)
	require.NoError(t, err)
	require.Equal(t, []string{first.ID, second.ID}, []string{otherAccounts[0].ID, otherAccounts[1].ID})
	require.NoError(t, fresh.SetOrder(ctx, f.other, []string{second.ID, first.ID}))
	require.Equal(t, ordered, list())
	otherAccounts, err = fresh.ListByUser(ctx, f.other)
	require.NoError(t, err)
	require.Equal(t, second.ID, otherAccounts[0].ID)
	save([]string{personal.ID, first.ID}, 409)
	save([]string{personal.ID, first.ID, first.ID}, 400)
	save([]string{personal.ID, second.ID, uuid.NewString()}, 403)
	foreign, err := svc.CreateAccount(ctx, f.other, model.CreateAccountReq{Name: "Foreign", AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, Currency: "USD", InitialBalanceDate: date})
	require.NoError(t, err)
	save([]string{personal.ID, second.ID, foreign.ID}, 403)
	require.Equal(t, ordered, list())
	newAccount := create("New", false)
	require.Equal(t, append(append([]string{}, ordered...), newAccount.ID), list())
	save(ordered, 409)
	// Soft deletion removes the account from reads and rejects stale reorder requests.
	require.NoError(t, svc.DeleteAccount(ctx, f.user, first.ID))
	save([]string{personal.ID, second.ID, first.ID, newAccount.ID}, 403)
	require.Equal(t, []string{personal.ID, second.ID, newAccount.ID}, list())
	// A database failure halfway through the write must leave every position intact.
	_, err = f.pool.Exec(ctx, `ALTER TABLE account_order ADD CONSTRAINT reject_first_position CHECK(position <> 0) NOT VALID`)
	require.NoError(t, err)
	save([]string{newAccount.ID, second.ID, personal.ID}, 500)
	require.Equal(t, []string{personal.ID, second.ID, newAccount.ID}, list())
}
