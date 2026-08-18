package analytics

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/co-wallet/backend/internal/model"
)

const dateLayout = "2006-01-02"

// filterParams представляет валидированные query-параметры эндпоинтов аналитики.
type filterParams struct {
	DateFrom     time.Time
	DateTo       time.Time
	AccountIDs   []string
	AccountKinds []model.AccountKind
	Currency     string
	TxType       model.TransactionType
}

// parseFilterParams читает query, валидирует значения и возвращает
// structured параметры. Пустые параметры — допустимы и заполняются
// дефолтами вызывающей стороной (currency из профиля пользователя).
func parseFilterParams(q url.Values) (filterParams, error) {
	p := filterParams{AccountKinds: []model.AccountKind{model.AccountKindCurrent}}

	now := time.Now()

	dateFromRaw := strings.TrimSpace(q.Get("date_from"))
	if dateFromRaw == "" {
		dateFromRaw = now.Format("2006-01") + "-01"
	}
	df, err := time.Parse(dateLayout, dateFromRaw)
	if err != nil {
		return filterParams{}, fmt.Errorf("date_from must be YYYY-MM-DD")
	}
	p.DateFrom = df

	dateToRaw := strings.TrimSpace(q.Get("date_to"))
	if dateToRaw == "" {
		dateToRaw = now.Format(dateLayout)
	}
	dt, err := time.Parse(dateLayout, dateToRaw)
	if err != nil {
		return filterParams{}, fmt.Errorf("date_to must be YYYY-MM-DD")
	}
	p.DateTo = dt

	if p.DateFrom.After(p.DateTo) {
		return filterParams{}, fmt.Errorf("date_from must be on or before date_to")
	}

	if raw := strings.TrimSpace(q.Get("account_ids")); raw != "" {
		for _, id := range strings.Split(raw, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, err := uuid.Parse(id); err != nil {
				return filterParams{}, fmt.Errorf("account_ids must contain valid UUIDs")
			}
			p.AccountIDs = append(p.AccountIDs, id)
		}
	}

	if raw := strings.TrimSpace(q.Get("account_kinds")); raw != "" {
		p.AccountKinds = nil
		if raw != "all" {
			seen := make(map[model.AccountKind]struct{})
			for _, value := range strings.Split(raw, ",") {
				kind := model.AccountKind(strings.TrimSpace(value))
				if !kind.IsValid() {
					return filterParams{}, fmt.Errorf("account_kinds must contain 'current', 'deposit', or 'investment'")
				}
				if _, exists := seen[kind]; exists {
					continue
				}
				seen[kind] = struct{}{}
				p.AccountKinds = append(p.AccountKinds, kind)
			}
		}
	}

	if cur := strings.TrimSpace(q.Get("currency")); cur != "" {
		cur = strings.ToUpper(cur)
		if len(cur) != 3 || !isAlpha(cur) {
			return filterParams{}, fmt.Errorf("currency must be a 3-letter ISO code")
		}
		p.Currency = cur
	}

	if t := strings.TrimSpace(q.Get("type")); t != "" {
		tt := model.TransactionType(t)
		if tt != model.TransactionTypeExpense && tt != model.TransactionTypeIncome {
			return filterParams{}, fmt.Errorf("type must be 'expense' or 'income'")
		}
		p.TxType = tt
	}

	return p, nil
}

func (p filterParams) toFilter(userID, defaultCurrency string) model.AnalyticsFilter {
	currency := p.Currency
	if currency == "" {
		currency = defaultCurrency
	}
	return model.AnalyticsFilter{
		UserID:          userID,
		DateFrom:        p.DateFrom,
		DateTo:          p.DateTo,
		AccountIDs:      p.AccountIDs,
		AccountKinds:    p.AccountKinds,
		DisplayCurrency: currency,
		TxType:          p.TxType,
	}
}

func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}
