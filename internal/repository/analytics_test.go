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

func TestBuildByCategoryQueryIncludesUncategorizedTransactions(t *testing.T) {
	query, _ := buildByCategoryQuery(model.AnalyticsFilter{})

	wantContains := []string{
		"COALESCE(c.id::text, 'uncategorized')",
		"COALESCE(c.name, 'Без категории')",
		"LEFT JOIN categories c ON c.id = t.category_id",
	}
	for _, substr := range wantContains {
		if !strings.Contains(query, substr) {
			t.Errorf("by-category query missing %q\n--- got ---\n%s", substr, query)
		}
	}
}

func TestTransactionFilterMatchesListSemantics(t *testing.T) {
	tests := []struct {
		name         string
		filter       model.AnalyticsFilter
		wantContains []string
		wantArgs     []any
		wantNext     int
	}{
		{
			name: "category and any tag",
			filter: model.AnalyticsFilter{
				CategoryIDs: []string{"category-1"},
				TagIDs:      []string{"tag-1", "tag-2"},
				TagMode:     "or",
			},
			wantContains: []string{
				"t.category_id = ANY($2)",
				"tt_filter.tag_id = ANY($3)",
			},
			wantArgs: []any{"user-id", []string{"category-1"}, []string{"tag-1", "tag-2"}},
			wantNext: 4,
		},
		{
			name: "all selected tags",
			filter: model.AnalyticsFilter{
				TagIDs:  []string{"tag-1", "tag-2"},
				TagMode: "and",
			},
			wantContains: []string{
				"tt_filter.tag_id = $2",
				"tt_filter.tag_id = $3",
			},
			wantArgs: []any{"user-id", "tag-1", "tag-2"},
			wantNext: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, args, next := transactionFilter(tt.filter, []any{"user-id"}, 2)
			for _, substr := range tt.wantContains {
				if !strings.Contains(condition, substr) {
					t.Errorf("transaction filter missing %q: %s", substr, condition)
				}
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("unexpected args: %#v", args)
			}
			if next != tt.wantNext {
				t.Fatalf("unexpected next placeholder: %d", next)
			}
		})
	}
}

func TestBuildByCategoryQueryIncludesTransactionFilters(t *testing.T) {
	query, args := buildByCategoryQuery(model.AnalyticsFilter{
		UserID:          "user-id",
		CategoryIDs:     []string{"category-1"},
		TagIDs:          []string{"tag-1"},
		TagMode:         "or",
		DisplayCurrency: "RUB",
	})

	for _, substr := range []string{
		"t.category_id = ANY($2)",
		"tt_filter.tag_id = ANY($3)",
		"quote_currency = $4",
		"t.type = $7",
	} {
		if !strings.Contains(query, substr) {
			t.Errorf("by-category query missing %q\n--- got ---\n%s", substr, query)
		}
	}
	if len(args) != 7 {
		t.Fatalf("unexpected args length: %d (%#v)", len(args), args)
	}
}
