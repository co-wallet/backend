package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/model"
)

type AnalyticsRepository struct {
	db *pgxpool.Pool
}

func NewAnalyticsRepository(db *pgxpool.Pool) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

// accountFilter builds a conditional snippet " AND a.id IN ($n,...)" and appends
// the account IDs to args, returning the updated args and next placeholder index.
func accountFilter(accountIDs []string, args []any, idx int) (string, []any, int) {
	if len(accountIDs) == 0 {
		return "", args, idx
	}
	ph := make([]string, len(accountIDs))
	for i, id := range accountIDs {
		ph[i] = fmt.Sprintf("$%d", idx)
		args = append(args, id)
		idx++
	}
	return fmt.Sprintf(" AND a.id IN (%s)", strings.Join(ph, ",")), args, idx
}

// convertExpr returns a SQL expression that converts `amount_expr` from `from_currency_col`
// to the display currency at position `displayCurrencyIdx`.
// Formula: amount * rate(USD→display) / rate(USD→from_currency)
func convertExpr(amountExpr, fromCurrencyCol string, displayCurrencyIdx int) string {
	return fmt.Sprintf(`
		%s
		* COALESCE((SELECT er_d.rate FROM exchange_rates er_d
		             WHERE er_d.base_currency = 'USD' AND er_d.quote_currency = $%d), 1.0)
		/ NULLIF(COALESCE((SELECT er_f.rate FROM exchange_rates er_f
		                   WHERE er_f.base_currency = 'USD' AND er_f.quote_currency = %s), 1.0), 0)`,
		amountExpr, displayCurrencyIdx, fromCurrencyCol)
}

func (r *AnalyticsRepository) Summary(ctx context.Context, f model.AnalyticsFilter) (model.AnalyticsSummary, error) {
	displayCurrency := f.DisplayCurrency
	if displayCurrency == "" {
		displayCurrency = "USD"
	}

	// --- Balance: initial balances + all-time income shares - expense shares, converted to display currency ---
	bArgs := []any{f.UserID}
	bIdx := 2
	bAcctCond, bArgs, bIdx := accountFilter(f.AccountIDs, bArgs, bIdx)

	// display currency is the next arg
	dispIdx := bIdx
	bArgs = append(bArgs, displayCurrency)

	balanceQuery := fmt.Sprintf(`
		WITH account_balances AS (
		    SELECT
		        a.currency,
		        a.initial_balance
		        + COALESCE(SUM(CASE WHEN t.type = 'income'  AND t.include_in_balance THEN ts.amount ELSE 0 END), 0)
		        - COALESCE(SUM(CASE WHEN t.type = 'expense' AND t.include_in_balance THEN ts.amount ELSE 0 END), 0)
		        AS balance_native
		    FROM accounts a
		    LEFT JOIN transactions t ON t.account_id = a.id
		    LEFT JOIN transaction_shares ts ON ts.transaction_id = t.id AND ts.user_id = $1
		    WHERE (a.owner_id = $1 OR EXISTS (
		              SELECT 1 FROM account_members am
		              WHERE am.account_id = a.id AND am.user_id = $1))%s
		      AND a.include_in_balance = true
		      AND a.deleted_at IS NULL
		    GROUP BY a.id, a.currency, a.initial_balance
		)
		SELECT COALESCE(SUM(%s), 0)
		FROM account_balances ab`,
		bAcctCond,
		convertExpr("ab.balance_native", "ab.currency", dispIdx),
	)

	var balance float64
	if err := r.db.QueryRow(ctx, balanceQuery, bArgs...).Scan(&balance); err != nil {
		return model.AnalyticsSummary{}, fmt.Errorf("balance query: %w", err)
	}

	// --- Expenses and income for the requested period, converted ---
	pArgs := []any{f.UserID}
	pIdx := 2
	pAcctCond, pArgs, pIdx := accountFilter(f.AccountIDs, pArgs, pIdx)

	pDispIdx := pIdx
	dateFromIdx := pIdx + 1
	dateToIdx := pIdx + 2
	pArgs = append(pArgs, displayCurrency, f.DateFrom, f.DateTo)

	pQuery := fmt.Sprintf(`
		SELECT
		    COALESCE(SUM(CASE WHEN t.type = 'expense' THEN %s ELSE 0 END), 0) AS expenses,
		    COALESCE(SUM(CASE WHEN t.type = 'income'  THEN %s ELSE 0 END), 0) AS income
		FROM transactions t
		JOIN transaction_shares ts ON ts.transaction_id = t.id AND ts.user_id = $1
		JOIN accounts a ON a.id = t.account_id
		WHERE (a.owner_id = $1 OR EXISTS (
		          SELECT 1 FROM account_members am
		          WHERE am.account_id = a.id AND am.user_id = $1))%s
		  AND a.deleted_at IS NULL
		  AND t.include_in_balance = true
		  AND t.date >= $%d::date
		  AND t.date <= $%d::date
		  AND t.type IN ('expense','income')`,
		convertExpr("ts.amount", "t.currency", pDispIdx),
		convertExpr("ts.amount", "t.currency", pDispIdx),
		pAcctCond,
		dateFromIdx,
		dateToIdx,
	)

	var expenses, income float64
	if err := r.db.QueryRow(ctx, pQuery, pArgs...).Scan(&expenses, &income); err != nil {
		return model.AnalyticsSummary{}, fmt.Errorf("period query: %w", err)
	}

	return model.AnalyticsSummary{Balance: balance, Expenses: expenses, Income: income}, nil
}

func (r *AnalyticsRepository) ByCategory(ctx context.Context, f model.AnalyticsFilter) ([]model.CategoryStat, error) {
	displayCurrency := f.DisplayCurrency
	if displayCurrency == "" {
		displayCurrency = "USD"
	}

	args := []any{f.UserID}
	idx := 2
	acctCond, args, idx := accountFilter(f.AccountIDs, args, idx)

	dispIdx := idx
	dateFrom := idx + 1
	dateTo := idx + 2
	args = append(args, displayCurrency, f.DateFrom, f.DateTo)

	q := fmt.Sprintf(`
		SELECT c.id, c.name, c.icon, COALESCE(SUM(%s), 0) AS amount
		FROM transactions t
		JOIN transaction_shares ts ON ts.transaction_id = t.id AND ts.user_id = $1
		JOIN accounts a ON a.id = t.account_id
		JOIN categories c ON c.id = t.category_id
		WHERE (a.owner_id = $1 OR EXISTS (
		          SELECT 1 FROM account_members am
		          WHERE am.account_id = a.id AND am.user_id = $1))%s
		  AND a.deleted_at IS NULL
		  AND t.include_in_balance = true
		  AND t.type = 'expense'
		  AND t.date >= $%d::date
		  AND t.date <= $%d::date
		GROUP BY c.id, c.name, c.icon
		ORDER BY amount DESC`,
		convertExpr("ts.amount", "t.currency", dispIdx),
		acctCond,
		dateFrom,
		dateTo,
	)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("by-category query: %w", err)
	}
	defer rows.Close()

	var result []model.CategoryStat
	for rows.Next() {
		var s model.CategoryStat
		if err := rows.Scan(&s.CategoryID, &s.CategoryName, &s.Icon, &s.Amount); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *AnalyticsRepository) ByTag(ctx context.Context, f model.AnalyticsFilter) ([]model.TagStat, error) {
	displayCurrency := f.DisplayCurrency
	if displayCurrency == "" {
		displayCurrency = "USD"
	}

	args := []any{f.UserID}
	idx := 2
	acctCond, args, idx := accountFilter(f.AccountIDs, args, idx)

	dispIdx := idx
	dateFrom := idx + 1
	dateTo := idx + 2
	args = append(args, displayCurrency, f.DateFrom, f.DateTo)

	q := fmt.Sprintf(`
		SELECT tg.id, tg.name, COALESCE(SUM(%s), 0) AS amount
		FROM transactions t
		JOIN transaction_shares ts ON ts.transaction_id = t.id AND ts.user_id = $1
		JOIN accounts a ON a.id = t.account_id
		JOIN transaction_tags tt ON tt.transaction_id = t.id
		JOIN tags tg ON tg.id = tt.tag_id
		WHERE (a.owner_id = $1 OR EXISTS (
		          SELECT 1 FROM account_members am
		          WHERE am.account_id = a.id AND am.user_id = $1))%s
		  AND a.deleted_at IS NULL
		  AND t.include_in_balance = true
		  AND t.type = 'expense'
		  AND t.date >= $%d::date
		  AND t.date <= $%d::date
		GROUP BY tg.id, tg.name
		ORDER BY amount DESC`,
		convertExpr("ts.amount", "t.currency", dispIdx),
		acctCond,
		dateFrom,
		dateTo,
	)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("by-tag query: %w", err)
	}
	defer rows.Close()

	var result []model.TagStat
	for rows.Next() {
		var s model.TagStat
		if err := rows.Scan(&s.TagID, &s.TagName, &s.Amount); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
