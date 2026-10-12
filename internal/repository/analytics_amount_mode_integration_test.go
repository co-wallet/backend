package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsAmountModes(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	exec := func(query string, args ...any) {
		_, err := f.pool.Exec(ctx, query, args...)
		require.NoError(t, err)
	}
	var shared, personal, foreign, deleted, tag string
	for _, account := range []struct {
		id            *string
		owner, access string
		initial       float64
	}{
		{&shared, f.user, "shared", 1000},
		{&personal, f.user, "personal", 100},
		{&foreign, f.other, "personal", 9000},
		{&deleted, f.user, "shared", 5000},
	} {
		require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,currency,access_mode,kind,initial_balance,initial_balance_date)
			VALUES($1,'Account','USD',$2,'spending',$3,CURRENT_DATE) RETURNING id`, account.owner, account.access, account.initial).Scan(account.id))
	}
	exec(`INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,0.5),($1,$3,0.5)`, shared, f.user, f.other)
	exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, deleted)
	require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO tags(user_id,name) VALUES($1,'Tag') RETURNING id`, f.user).Scan(&tag))
	for _, kind := range []string{"expense", "income"} {
		var id string
		require.NoError(t, f.pool.QueryRow(ctx, `INSERT INTO transactions(account_id,type,amount,currency,date,created_by,default_currency,default_currency_amount)
			VALUES($1,$2,100,'USD',CURRENT_DATE,$3,'RUB',8000) RETURNING id`, shared, kind, f.user).Scan(&id))
		// A custom historical 25/75 split must override the account's 50/50 defaults.
		exec(`INSERT INTO transaction_shares(transaction_id,user_id,amount,is_custom) VALUES($1,$2,25,true),($1,$3,75,true)`, id, f.user, f.other)
		exec(`INSERT INTO transaction_tags(transaction_id,tag_id) VALUES($1,$2)`, id, tag)
	}
	exec(`INSERT INTO exchange_rates(base_currency,quote_currency,rate) VALUES('USD','RUB',90),('USD','EUR',0.8)
		ON CONFLICT(base_currency,quote_currency) DO UPDATE SET rate=EXCLUDED.rate`)
	analytics := repository.NewAnalyticsRepository(f.pool)
	filter := model.AnalyticsFilter{UserID: f.user, AccountIDs: []string{shared, personal, foreign, deleted},
		DateFrom: time.Now().AddDate(0, 0, -1), DateTo: time.Now().AddDate(0, 0, 1)}
	for _, tc := range []struct {
		mode            model.AmountMode
		currency        string
		balance, amount float64
	}{
		{"", "USD", 600, 25},
		{model.AmountModeShare, "USD", 600, 25},
		{model.AmountModeTotal, "USD", 1100, 100},
		{model.AmountModeShare, "RUB", 54000, 2000},
		{model.AmountModeTotal, "RUB", 99000, 8000},
		{model.AmountModeShare, "EUR", 480, 20},
		{model.AmountModeTotal, "EUR", 880, 80},
	} {
		t.Run(string(tc.mode)+"/"+tc.currency, func(t *testing.T) {
			filter.AmountMode, filter.DisplayCurrency = tc.mode, tc.currency
			summary, err := analytics.Summary(ctx, filter)
			require.NoError(t, err)
			require.InDelta(t, tc.balance, summary.Balance, 1e-8)
			require.InDelta(t, tc.amount, summary.Expenses, 1e-8)
			require.InDelta(t, tc.amount, summary.Income, 1e-8)
			for _, kind := range []model.TransactionType{model.TransactionTypeExpense, model.TransactionTypeIncome} {
				filter.TxType = kind
				categories, err := analytics.ByCategory(ctx, filter)
				require.NoError(t, err)
				require.Len(t, categories, 1)
				require.InDelta(t, tc.amount, categories[0].Amount, 1e-8)
				tags, err := analytics.ByTag(ctx, filter)
				require.NoError(t, err)
				require.Len(t, tags, 1)
				require.InDelta(t, tc.amount, tags[0].Amount, 1e-8)
			}
		})
	}
	filter.AmountMode, filter.DisplayCurrency = model.AmountModeTotal, "USD"
	filter.AccountIDs = []string{foreign, deleted}
	summary, err := analytics.Summary(ctx, filter)
	require.NoError(t, err)
	require.Zero(t, summary.Balance)
	require.Zero(t, summary.Expenses)
	filter.AccountIDs = []string{shared}
	filter.TagIDs = []string{f.user} // valid UUID, but no matching tag
	summary, err = analytics.Summary(ctx, filter)
	require.NoError(t, err)
	require.Zero(t, summary.Expenses)
	require.Zero(t, summary.Income)
	filter.TagIDs = nil
	filter.DisplayCurrency = "TRY" // no usable current rate
	summary, err = analytics.Summary(ctx, filter)
	require.NoError(t, err)
	require.Zero(t, summary.Expenses)
	require.Equal(t, 1, summary.ExpensesMissingAmounts)
	require.Equal(t, 1, summary.IncomeMissingAmounts)
	// A member with a zero historical share still sees the full operation in total mode.
	exec(`UPDATE transaction_shares SET amount=CASE WHEN user_id=$1 THEN 0 ELSE 100 END`, f.user)
	filter.DisplayCurrency = "USD"
	for _, tc := range []struct {
		mode model.AmountMode
		want float64
	}{{model.AmountModeShare, 0}, {model.AmountModeTotal, 100}} {
		filter.AmountMode = tc.mode
		summary, err = analytics.Summary(ctx, filter)
		require.NoError(t, err)
		require.Equal(t, tc.want, summary.Expenses)
		require.Equal(t, tc.want, summary.Income)
	}
	filter.AmountMode = model.AmountModeTotal
	filter.UserID = f.other // membership grants the same full view, without ownership
	summary, err = analytics.Summary(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, float64(100), summary.Expenses)
	filter.AccountKinds = []model.AccountKind{model.AccountKindDeposit}
	summary, err = analytics.Summary(ctx, filter)
	require.NoError(t, err)
	require.Zero(t, summary.Balance)
	require.Zero(t, summary.Expenses)
}
