package repository

import (
	"strings"
	"testing"
)

// TestConvertExprWrapsAmountInParens гарантирует, что составные выражения
// (сумма/разность) не ломают приоритет операторов SQL: весь amountExpr должен
// быть обёрнут в скобки, чтобы умножение/деление на курсы применялось к
// результату целиком, а не только к последнему слагаемому.
func TestConvertExprWrapsAmountInParens(t *testing.T) {
	tests := []struct {
		name            string
		amountExpr      string
		fromCurrencyCol string
		displayIdx      int
		wantContains    []string
	}{
		{
			name:            "compound expression is wrapped",
			amountExpr:      "ab.balance_native + COALESCE(ti.amount, 0)",
			fromCurrencyCol: "ab.currency",
			displayIdx:      3,
			wantContains: []string{
				"(ab.balance_native + COALESCE(ti.amount, 0))",
				"quote_currency = $3",
				"quote_currency = ab.currency",
			},
		},
		{
			name:            "simple column is still wrapped (harmless)",
			amountExpr:      "ts.amount",
			fromCurrencyCol: "t.currency",
			displayIdx:      2,
			wantContains: []string{
				"(ts.amount)",
				"quote_currency = $2",
				"quote_currency = t.currency",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertExpr(tt.amountExpr, tt.fromCurrencyCol, tt.displayIdx)
			for _, substr := range tt.wantContains {
				if !strings.Contains(got, substr) {
					t.Errorf("convertExpr output missing %q\n--- got ---\n%s", substr, got)
				}
			}
		})
	}
}
