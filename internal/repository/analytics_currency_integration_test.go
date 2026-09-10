package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/model"
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
		require.Equal(t, want, summary.Expenses)
		require.Equal(t, want, summary.Income)
		for _, kind := range []model.TransactionType{model.TransactionTypeExpense, model.TransactionTypeIncome} {
			filter.TxType = kind
			categories, err := analytics.ByCategory(ctx, filter)
			require.NoError(t, err)
			require.Len(t, categories, 1)
			require.Equal(t, want, categories[0].Amount)
			tags, err := analytics.ByTag(ctx, filter)
			require.NoError(t, err)
			require.Len(t, tags, 1)
			require.Equal(t, want, tags[0].Amount)
		}
	}
	for _, rate := range []int{90, 120} {
		exec(`INSERT INTO exchange_rates(base_currency,quote_currency,rate) VALUES('USD','RUB',$1)
			ON CONFLICT(base_currency,quote_currency) DO UPDATE SET rate=EXCLUDED.rate`, rate)
		check(2000)
	}
	exec(`UPDATE transactions SET default_currency_amount=12000 WHERE account_id=$1`, account)
	check(3000)
	filter.DisplayCurrency = "USD"
	check(25)
	filter.DisplayCurrency = "RUB"
	exec(`UPDATE transactions SET default_currency_amount=NULL WHERE account_id=$1 AND type='expense'`, account)
	partial, err := analytics.Summary(ctx, filter)
	require.NoError(t, err)
	require.Zero(t, partial.Expenses)
	require.Equal(t, 1, partial.ExpensesMissingAmounts)
	require.Zero(t, partial.IncomeMissingAmounts)
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
