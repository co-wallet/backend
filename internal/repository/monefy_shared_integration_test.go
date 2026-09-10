package repository_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
	"github.com/stretchr/testify/require"
)

func sharedPreview(t *testing.T, f *importFixture, ratio float64) model.ImportPreview {
	t.Helper()
	p := f.configured(t)
	access := map[string]model.ImportAccountAccess{}
	kinds := map[string]model.AccountKind{}
	for _, a := range p.Accounts {
		kinds[a.SourceID] = a.Kind
		if a.SourceID != "reserve" {
			access[a.SourceID] = model.ImportAccountAccess{AccessMode: "shared", Members: []model.CreateAccountMemberReq{{Username: "importer", DefaultShare: ratio}, {Username: "other", DefaultShare: 1 - ratio}}}
		}
	}
	configured, err := f.svc.Configure(context.Background(), f.user, p.ID, kinds, nil, nil, access)
	require.NoError(t, err)
	require.True(t, configured.Report.CanImport(), configured.Report.Diagnostics)
	require.Equal(t, model.AccountAccessModePersonal, p.Accounts[0].AccessMode)
	return configured
}

func TestMonefySharedHistoryBalancesAndImmutability(t *testing.T) {
	for _, ratio := range []float64{.5, .3333, 0, 1} {
		t.Run(strconv.FormatFloat(ratio, 'f', 4, 64), func(t *testing.T) {
			f := newImportFixture(t)
			ctx := context.Background()
			p := sharedPreview(t, f, ratio)
			result, err := f.svc.Confirm(ctx, f.user, p.ID, true, false)
			require.NoError(t, err)
			again, err := f.svc.Confirm(ctx, f.user, p.ID, true, false)
			require.NoError(t, err)
			require.Equal(t, result, again)
			var inconsistent int
			require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM transactions t WHERE t.amount <> (SELECT sum(s.amount) FROM transaction_shares s WHERE s.transaction_id=t.id)`).Scan(&inconsistent))
			require.Zero(t, inconsistent)
			repo := repository.NewAccountRepository(f.pool)
			for _, user := range []string{f.user, f.other} {
				balances, err := repo.ListBalancesByUser(ctx, user, "RUB")
				require.NoError(t, err)
				accounts, err := repo.ListByUser(ctx, user)
				require.NoError(t, err)
				for _, a := range accounts {
					for i, source := range p.Report.Accounts {
						if a.Name != source.Name {
							continue
						}
						for _, member := range p.Accounts[i].Members {
							if member.UserID != user {
								continue
							}
							want, err := strconv.ParseFloat(member.FinalBalance, 64)
							require.NoError(t, err)
							require.InDelta(t, want, balances[a.ID].BalanceNative, 1e-7)
						}
						require.Equal(t, p.Accounts[i].AccessMode, a.AccessMode)
					}
					summary, err := repository.NewAnalyticsRepository(f.pool).Summary(ctx, model.AnalyticsFilter{UserID: user, AccountIDs: []string{a.ID}, DisplayCurrency: a.Currency, DateFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), DateTo: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)})
					require.NoError(t, err)
					require.InDelta(t, balances[a.ID].BalanceNative, summary.Balance, 1e-7)
					accountSvc := service.NewAccountService(f.pool, repo, repository.NewUserRepository(f.pool))
					_, err = accountSvc.AddMember(ctx, a.ID, "other", .5)
					require.Error(t, err)
					_, err = accountSvc.UpdateMember(ctx, a.ID, f.other, .1)
					require.Error(t, err)
					require.Error(t, accountSvc.RemoveMember(ctx, user, a.ID, f.other))
				}
			}
		})
	}
}

func TestMonefySharedRollbackAndUnavailableMember(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	p := sharedPreview(t, f, .6)
	_, err := f.pool.Exec(ctx, `UPDATE users SET is_active=false WHERE id=$1`, f.other)
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, true, false)
	require.ErrorContains(t, err, "members_changed")
	_, err = f.pool.Exec(ctx, `UPDATE users SET is_active=true WHERE id=$1`, f.other)
	require.NoError(t, err)
	_, err = f.pool.Exec(ctx, `ALTER TABLE transactions ADD CONSTRAINT fail_shared_transfer CHECK(type<>'transfer')`)
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, true, false)
	require.Error(t, err)
	var count int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM accounts)+(SELECT count(*) FROM account_members)+(SELECT count(*) FROM transaction_shares)+(SELECT count(*) FROM monefy_imports)`).Scan(&count))
	require.Zero(t, count)
	_, err = f.pool.Exec(ctx, `ALTER TABLE transactions DROP CONSTRAINT fail_shared_transfer`)
	require.NoError(t, err)
	_, err = f.svc.Confirm(ctx, f.user, p.ID, true, false)
	require.NoError(t, err)
}
