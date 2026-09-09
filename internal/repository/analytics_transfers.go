package repository

import (
	"context"
	"fmt"

	"github.com/co-wallet/backend/internal/model"
)

// transferTotals считает только переводы через границу выбранного набора.
// Исходящая сумма — сохранённая доля пользователя; входящая — доля
// зачисления в валюте получателя, как в расчёте баланса счёта.
func (r *AnalyticsRepository) transferTotals(ctx context.Context, f model.AnalyticsFilter) (float64, float64, error) {
	args := []any{f.UserID}
	accountCond, args, idx := accountFilter(f.AccountIDs, args, 2)
	kindCond, args, idx := accountKindFilter(f.AccountKinds, args, idx)
	txCond, args, idx := transactionFilter(f, args, idx)
	args = append(args, f.DisplayCurrency, f.DateFrom, f.DateTo)
	query := fmt.Sprintf(`
        WITH selected_accounts AS (
            SELECT a.id, a.currency,
                COALESCE((SELECT am.default_share FROM account_members am
                    WHERE am.account_id = a.id AND am.user_id = $1), 1.0) AS share
            FROM accounts a
            WHERE a.deleted_at IS NULL
                AND (a.owner_id = $1 OR EXISTS (SELECT 1 FROM account_members am
                    WHERE am.account_id = a.id AND am.user_id = $1))%s%s
        )
        SELECT
            COALESCE(SUM(CASE WHEN source.id IS NOT NULL AND destination.id IS NULL
                THEN %s ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN destination.id IS NOT NULL AND source.id IS NULL
                THEN %s ELSE 0 END), 0)
        FROM transactions t
        LEFT JOIN selected_accounts source ON source.id = t.account_id
        LEFT JOIN selected_accounts destination ON destination.id = t.to_account_id
        LEFT JOIN transaction_shares ts ON ts.transaction_id = t.id AND ts.user_id = $1
        WHERE t.type = 'transfer'
            AND (source.id IS NOT NULL OR destination.id IS NOT NULL)
            AND t.date >= $%d::date AND t.date <= $%d::date%s`,
		accountCond, kindCond,
		convertExpr("COALESCE(ts.amount, 0)", "t.currency", idx),
		convertExpr("COALESCE(t.to_amount, t.amount) * destination.share", "destination.currency", idx),
		idx+1, idx+2, txCond,
	)
	var outgoing, incoming float64
	if err := r.db.QueryRow(ctx, query, args...).Scan(&outgoing, &incoming); err != nil {
		return 0, 0, fmt.Errorf("transfer totals query: %w", err)
	}
	return outgoing, incoming, nil
}
