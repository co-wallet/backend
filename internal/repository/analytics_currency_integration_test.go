package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsUsesSavedCurrencyAmounts(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	exec := func(query string, args ...any) {
		_, err := f.pool.Exec(ctx, query, args...)
		require.NoError(t, err)
	}
	var account, tag string
	require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance_date)
		VALUES($1,'Shared USD','USD','shared','spending',CURRENT_DATE) RETURNING id`, f.user).Scan(&account))
	exec(`INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,0.9)`, account, f.user)
	require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO tags(user_id,name) VALUES($1,'Tagged') RETURNING id`, f.user).Scan(&tag))
	for _, kind := range []string{"expense", "income"} {
		var id string
		require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO transactions(account_id,type,amount,currency,date,created_by,default_currency,default_currency_amount)
			VALUES($1,$2,100,'USD',CURRENT_DATE,$3,'RUB',8000) RETURNING id`, account, kind, f.user).Scan(&id))
		exec(`INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES($1,$2,25)`, id, f.user)
		exec(`INSERT INTO transaction_tags(transaction_id,tag_id) VALUES($1,$2)`, id, tag)
	}
	analytics := repository.NewAnalyticsRepository(f.pool)
	filter := model.AnalyticsFilter{UserID: f.user, DisplayCurrency: "RUB", DateFrom: time.Now().AddDate(0, 0, -1), DateTo: time.Now().AddDate(0, 0, 1), AccountIDs: []string{account}, TagIDs: []string{tag}}
	check := func(want float64) {
		t.Helper()
		summary, err := analytics.Summary(ctx, filter)
		require.NoError(t, err)
		require.InDelta(t, want, summary.Expenses, 1e-9)
		require.InDelta(t, want, summary.Income, 1e-9)
		require.Zero(t, summary.ExpensesMissingAmounts)
		require.Zero(t, summary.IncomeMissingAmounts)
		for _, kind := range []model.TransactionType{model.TransactionTypeExpense, model.TransactionTypeIncome} {
			filter.TxType = kind
			categories, err := analytics.ByCategory(ctx, filter)
			require.NoError(t, err)
			require.Len(t, categories, 1)
			require.InDelta(t, want, categories[0].Amount, 1e-9)
			require.Zero(t, categories[0].MissingAmounts)
			tags, err := analytics.ByTag(ctx, filter)
			require.NoError(t, err)
			require.Len(t, tags, 1)
			require.InDelta(t, want, tags[0].Amount, 1e-9)
			require.Zero(t, tags[0].MissingAmounts)
		}
	}
	for _, rate := range []int{90, 120} {
		exec(`INSERT INTO exchange_rates(base_currency,quote_currency,rate) VALUES('USD','RUB',$1)
			ON CONFLICT(base_currency,quote_currency) DO UPDATE SET rate=EXCLUDED.rate`, rate)
		check(2000)
	}
	filter.DisplayCurrency = "EUR"
	for _, rate := range []float64{0.9, 1.1} {
		exec(`INSERT INTO exchange_rates(base_currency,quote_currency,rate) VALUES('USD','EUR',$1)
			ON CONFLICT(base_currency,quote_currency) DO UPDATE SET rate=EXCLUDED.rate`, rate)
		check(25 * rate)
	}
	filter.DisplayCurrency = "RUB"
	exec(`UPDATE transactions SET default_currency_amount=12000 WHERE account_id=$1`, account)
	check(3000)
	filter.DisplayCurrency = "USD"
	check(25)
	filter.DisplayCurrency = "RUB"
	exec(`UPDATE transactions SET default_currency_amount=NULL WHERE account_id=$1 AND type='expense'`, account)
	check(3000)
	exec(`DELETE FROM exchange_rates WHERE base_currency='USD' AND quote_currency='RUB'`)
	partial, err := analytics.Summary(ctx, filter)
	require.NoError(t, err)
	require.Zero(t, partial.Expenses)
	require.Equal(t, 1, partial.ExpensesMissingAmounts)
	require.Zero(t, partial.IncomeMissingAmounts)
	require.Equal(t, float64(3000), partial.Income)
	filter.TxType = model.TransactionTypeExpense
	categories, err := analytics.ByCategory(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, 1, categories[0].MissingAmounts)
	tags, err := analytics.ByTag(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, 1, tags[0].MissingAmounts)
	filter.UserID = f.other
	summary, err := analytics.Summary(ctx, filter)
	require.NoError(t, err)
	require.Zero(t, summary.Expenses)
	require.Zero(t, summary.Income)
}

func TestAnalyticsCurrentCurrencyFallback(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	exec := func(query string, args ...any) {
		_, err := f.pool.Exec(ctx, query, args...)
		require.NoError(t, err)
	}
	var account string
	require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance_date)
		VALUES($1,'Shared account','EUR','shared','spending',CURRENT_DATE) RETURNING id`, f.user).Scan(&account))
	exec(`INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,0.9)`, account, f.user)
	for _, kind := range []string{"expense", "income"} {
		var id string
		require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO transactions(account_id,type,amount,currency,date,created_by,default_currency,default_currency_amount)
			VALUES($1,$2,100,'EUR',CURRENT_DATE,$3,'RUB',8000) RETURNING id`, account, kind, f.user).Scan(&id))
		exec(`INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES($1,$2,25)`, id, f.user)
	}
	analytics := repository.NewAnalyticsRepository(f.pool)
	filter := model.AnalyticsFilter{UserID: f.user, AccountIDs: []string{account}, DateFrom: time.Now().AddDate(0, 0, -1), DateTo: time.Now().AddDate(0, 0, 1)}
	for _, tc := range []struct {
		name, source, target   string
		sourceRate, targetRate *float64
		share, want            float64
		missing                int
	}{
		{name: "USD source needs no stored rate", source: "USD", target: "EUR", targetRate: ptr.To(0.9), share: 25, want: 22.5},
		{name: "USD target needs no stored rate", source: "EUR", target: "USD", sourceRate: ptr.To(0.8), share: 25, want: 31.25},
		{name: "cross rate ignores saved RUB amount", source: "EUR", target: "TRY", sourceRate: ptr.To(0.8), targetRate: ptr.To(40.0), share: 25, want: 1250},
		{name: "native amount needs no rates", source: "EUR", target: "EUR", share: 25, want: 25},
		{name: "saved amount needs no rates", source: "EUR", target: "RUB", share: 25, want: 2000},
		{name: "missing source rate", source: "EUR", target: "USD", share: 25, missing: 1},
		{name: "missing target rate", source: "USD", target: "EUR", share: 25, missing: 1},
		{name: "zero source rate", source: "EUR", target: "USD", sourceRate: ptr.To(0.0), share: 25, missing: 1},
		{name: "negative target rate", source: "USD", target: "EUR", targetRate: ptr.To(-1.0), share: 25, missing: 1},
		{name: "zero share needs no rates", source: "EUR", target: "USD", share: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec(`DELETE FROM exchange_rates`)
			for currency, rate := range map[string]*float64{tc.source: tc.sourceRate, tc.target: tc.targetRate} {
				if rate != nil {
					exec(`INSERT INTO exchange_rates(base_currency,quote_currency,rate) VALUES('USD',$1,$2)`, currency, *rate)
				}
			}
			exec(`UPDATE accounts SET currency=$1 WHERE id=$2`, tc.source, account)
			exec(`UPDATE transactions SET currency=$1 WHERE account_id=$2`, tc.source, account)
			exec(`UPDATE transaction_shares SET amount=$1 WHERE transaction_id IN (SELECT id FROM transactions WHERE account_id=$2)`, tc.share, account)
			filter.DisplayCurrency = tc.target
			summary, err := analytics.Summary(ctx, filter)
			require.NoError(t, err)
			require.Equal(t, tc.want, summary.Expenses)
			require.Equal(t, tc.want, summary.Income)
			require.Equal(t, tc.missing, summary.ExpensesMissingAmounts)
			require.Equal(t, tc.missing, summary.IncomeMissingAmounts)
			for _, kind := range []model.TransactionType{model.TransactionTypeExpense, model.TransactionTypeIncome} {
				filter.TxType = kind
				categories, err := analytics.ByCategory(ctx, filter)
				require.NoError(t, err)
				require.Len(t, categories, 1)
				require.Equal(t, tc.want, categories[0].Amount)
				require.Equal(t, tc.missing, categories[0].MissingAmounts)
				tags, err := analytics.ByTag(ctx, filter)
				require.NoError(t, err)
				require.Len(t, tags, 1)
				require.Equal(t, "untagged", tags[0].TagID)
				require.Equal(t, tc.want, tags[0].Amount)
				require.Equal(t, tc.missing, tags[0].MissingAmounts)
			}
			var savedAmount float64
			require.NoError(t, f.pool.QueryRow(ctx, `SELECT default_currency_amount FROM transactions WHERE account_id=$1 LIMIT 1`, account).Scan(&savedAmount))
			require.Equal(t, float64(8000), savedAmount)
		})
	}
}
