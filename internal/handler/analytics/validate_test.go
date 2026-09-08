package analytics

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/co-wallet/backend/internal/model"
)

func TestParseFilterParams(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	secondUUID := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	tests := []struct {
		name    string
		query   url.Values
		wantErr string
		check   func(t *testing.T, p filterParams)
	}{
		{
			name:  "defaults when empty",
			query: url.Values{},
			check: func(t *testing.T, p filterParams) {
				now := time.Now()
				assert.Equal(t, now.Format("2006-01")+"-01", p.DateFrom.Format(dateLayout))
				assert.Equal(t, now.Format(dateLayout), p.DateTo.Format(dateLayout))
				assert.Nil(t, p.AccountIDs)
				assert.Equal(t, []model.AccountKind{model.AccountKindSpending}, p.AccountKinds)
				assert.Empty(t, p.Currency)
				assert.Empty(t, string(p.TxType))
			},
		},
		{
			name: "valid full payload",
			query: url.Values{
				"date_from":     {"2026-01-01"},
				"date_to":       {"2026-01-31"},
				"account_ids":   {validUUID + "," + validUUID},
				"account_kinds": {"spending,investment,spending"},
				"category_ids":  {validUUID},
				"tag_ids":       {validUUID + "," + secondUUID},
				"tag_mode":      {"and"},
				"currency":      {"eur"},
				"type":          {"income"},
			},
			check: func(t *testing.T, p filterParams) {
				assert.Equal(t, "2026-01-01", p.DateFrom.Format(dateLayout))
				assert.Equal(t, "2026-01-31", p.DateTo.Format(dateLayout))
				assert.Equal(t, []string{validUUID, validUUID}, p.AccountIDs)
				assert.Equal(t, []model.AccountKind{model.AccountKindSpending, model.AccountKindInvestment}, p.AccountKinds)
				assert.Equal(t, []string{validUUID}, p.CategoryIDs)
				assert.Equal(t, []string{validUUID, secondUUID}, p.TagIDs)
				assert.Equal(t, "and", p.TagMode)
				assert.Equal(t, "EUR", p.Currency)
				assert.Equal(t, model.TransactionTypeIncome, p.TxType)
			},
		},
		{
			name:  "all account kinds removes kind restriction",
			query: url.Values{"account_kinds": {"all"}},
			check: func(t *testing.T, p filterParams) {
				assert.Nil(t, p.AccountKinds)
			},
		},
		{
			name:    "invalid account kind",
			query:   url.Values{"account_kinds": {"spending,crypto"}},
			wantErr: "account_kinds must contain 'spending', 'deposit', or 'investment'",
		},
		{
			name:    "invalid date_from",
			query:   url.Values{"date_from": {"01-01-2026"}},
			wantErr: "date_from must be YYYY-MM-DD",
		},
		{
			name:    "invalid date_to",
			query:   url.Values{"date_to": {"not-a-date"}},
			wantErr: "date_to must be YYYY-MM-DD",
		},
		{
			name: "date_from after date_to",
			query: url.Values{
				"date_from": {"2026-02-01"},
				"date_to":   {"2026-01-01"},
			},
			wantErr: "date_from must be on or before date_to",
		},
		{
			name:    "invalid account_id",
			query:   url.Values{"account_ids": {"not-a-uuid"}},
			wantErr: "account_ids must contain valid UUIDs",
		},
		{
			name:    "mixed valid and invalid account_ids",
			query:   url.Values{"account_ids": {validUUID + ",bad"}},
			wantErr: "account_ids must contain valid UUIDs",
		},
		{
			name:    "invalid category_id",
			query:   url.Values{"category_ids": {"not-a-uuid"}},
			wantErr: "category_ids must contain valid UUIDs",
		},
		{
			name:    "invalid tag_id",
			query:   url.Values{"tag_ids": {validUUID + ",bad"}},
			wantErr: "tag_ids must contain valid UUIDs",
		},
		{
			name:    "invalid tag mode",
			query:   url.Values{"tag_mode": {"xor"}},
			wantErr: "tag_mode must be 'or' or 'and'",
		},
		{
			name:    "currency wrong length",
			query:   url.Values{"currency": {"EURO"}},
			wantErr: "currency must be a 3-letter ISO code",
		},
		{
			name:    "currency with digits",
			query:   url.Values{"currency": {"E1R"}},
			wantErr: "currency must be a 3-letter ISO code",
		},
		{
			name:    "invalid type",
			query:   url.Values{"type": {"transfer"}},
			wantErr: "type must be 'expense' or 'income'",
		},
		{
			name:  "empty account_ids items skipped",
			query: url.Values{"account_ids": {" , , "}},
			check: func(t *testing.T, p filterParams) {
				assert.Empty(t, p.AccountIDs)
			},
		},
		{
			name:  "tag mode defaults to or",
			query: url.Values{},
			check: func(t *testing.T, p filterParams) {
				assert.Equal(t, "or", p.TagMode)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parseFilterParams(tt.query)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			if tt.check != nil {
				tt.check(t, p)
			}
		})
	}
}

func TestFilterParams_ToFilter(t *testing.T) {
	df, _ := time.Parse(dateLayout, "2026-03-01")
	dt, _ := time.Parse(dateLayout, "2026-03-31")

	t.Run("uses explicit currency when provided", func(t *testing.T) {
		p := filterParams{
			DateFrom:    df,
			DateTo:      dt,
			CategoryIDs: []string{"category-1"},
			TagIDs:      []string{"tag-1"},
			TagMode:     "and",
			Currency:    "EUR",
			TxType:      model.TransactionTypeExpense,
		}
		f := p.toFilter("user-1", "USD")
		assert.Equal(t, "EUR", f.DisplayCurrency)
		assert.Equal(t, "user-1", f.UserID)
		assert.Equal(t, df, f.DateFrom)
		assert.Equal(t, dt, f.DateTo)
		assert.Equal(t, model.TransactionTypeExpense, f.TxType)
		assert.Equal(t, p.AccountKinds, f.AccountKinds)
		assert.Equal(t, p.CategoryIDs, f.CategoryIDs)
		assert.Equal(t, p.TagIDs, f.TagIDs)
		assert.Equal(t, "and", f.TagMode)
	})

	t.Run("falls back to default currency", func(t *testing.T) {
		p := filterParams{DateFrom: df, DateTo: dt}
		f := p.toFilter("user-1", "USD")
		assert.Equal(t, "USD", f.DisplayCurrency)
	})
}
