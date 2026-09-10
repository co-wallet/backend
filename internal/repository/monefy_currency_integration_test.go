package repository_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/stretchr/testify/require"
)

func TestMonefyFallbackCurrencyRate(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(map[bool]string{false: "current snapshot", true: "manual snapshot"}[manual], func(t *testing.T) {
			f := newImportFixture(t)
			ctx := context.Background()
			source, err := sql.Open("sqlite", f.source)
			require.NoError(t, err)
			// No applicable rate for the income on Jan 1; the transfer on Jan 3 still has one.
			_, err = source.Exec(`DELETE FROM CurrencyRate WHERE Id='earlier'; UPDATE "Transaction" SET CreatedOn=638396640000000000 WHERE Id='income'`)
			require.NoError(t, err)
			require.NoError(t, source.Close())
			_, err = f.pool.Exec(ctx, `INSERT INTO exchange_rates(base_currency,quote_currency,rate) VALUES('USD','RUB',90),('USD','TRY',3)
				ON CONFLICT(base_currency,quote_currency) DO UPDATE SET rate=EXCLUDED.rate`)
			require.NoError(t, err)
			p := f.configured(t)
			require.Equal(t, "30.000000000000", p.CurrencyRates["TRY"].Rate)
			require.Equal(t, "current", p.CurrencyRates["TRY"].Source)
			require.Equal(t, 1, p.CurrencyRates["TRY"].Transactions)
			want := "29999.9700"
			if manual {
				for _, invalid := range []map[string]string{{"TRY": "0"}, {"TRY": "-1"}, {"TRY": "1/2"}, {"TRY": "NaN"}, {"USD": "2"}} {
					_, err := f.svc.ConfigureRates(ctx, f.user, p.ID, invalid)
					require.ErrorIs(t, err, apperr.ErrValidation)
				}
				_, err = f.svc.ConfigureRates(ctx, f.other, p.ID, map[string]string{"TRY": "25"})
				require.ErrorIs(t, err, apperr.ErrNotFound)
				updated, err := f.svc.ConfigureRates(ctx, f.user, p.ID, map[string]string{"TRY": "25"})
				require.NoError(t, err)
				require.NotEqual(t, p.ID, updated.ID)
				require.Equal(t, "manual", updated.CurrencyRates["TRY"].Source)
				original, err := f.store.Load(f.user, p.ID)
				require.NoError(t, err)
				require.Equal(t, "30.000000000000", original.CurrencyRates["TRY"].Rate)
				p = updated
				want = "24999.9750"
			}
			// A rate update and unrelated option change must not change reviewed amounts.
			_, err = f.pool.Exec(ctx, `UPDATE exchange_rates SET rate=120 WHERE quote_currency='RUB'`)
			require.NoError(t, err)
			p, err = f.svc.Configure(ctx, f.user, p.ID, map[string]model.AccountKind{"cash": "spending", "travel": "deposit", "reserve": "investment"}, nil, nil, nil)
			require.NoError(t, err)
			_, err = f.svc.Confirm(ctx, f.user, p.ID, false, false)
			require.NoError(t, err)
			var currency, amount string
			require.NoError(t, f.pool.QueryRow(ctx, `SELECT default_currency,default_currency_amount::text FROM transactions WHERE type='income'`).Scan(&currency, &amount))
			require.Equal(t, "RUB", currency)
			require.Equal(t, want, amount)
		})
	}
}

func TestMonefyPersistsHistoricalBaseAmount(t *testing.T) {
	f := newImportFixture(t)
	p := f.configured(t)
	require.Empty(t, p.CurrencyRates)
	_, err := f.svc.Confirm(context.Background(), f.user, p.ID, false, false)
	require.NoError(t, err)
	var amount string
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT default_currency_amount::text FROM transactions WHERE type='income'`).Scan(&amount))
	require.Equal(t, "3000.0000", amount)
}
