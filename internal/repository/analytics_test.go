package repository

import (
	"reflect"
	"strings"
	"testing"

	"github.com/co-wallet/backend/internal/model"
)

func TestAccountKindFilter(t *testing.T) {
	condition, args, next := accountKindFilter(
		[]model.AccountKind{model.AccountKindSpending, model.AccountKindInvestment},
		[]any{"user-id"},
		2,
	)

	if condition != " AND a.kind IN ($2,$3)" {
		t.Fatalf("unexpected condition: %q", condition)
	}
	wantArgs := []any{"user-id", model.AccountKindSpending, model.AccountKindInvestment}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("unexpected args: %#v", args)
	}
	if next != 4 {
		t.Fatalf("unexpected next placeholder: %d", next)
	}
}

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
