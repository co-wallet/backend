package service

import (
	"context"
	"maps"
	"math/big"
	"regexp"
	"slices"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/model"
	"github.com/google/uuid"
)

var importRatePattern = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,12})?$`)

// ConfigureRates changes only fallback rates; historical source values remain intact.
func (s *ImportService) ConfigureRates(ctx context.Context, user, id string, rates map[string]string) (model.ImportPreview, error) {
	p, err := s.store.Load(user, id)
	if err != nil {
		return model.ImportPreview{}, err
	}
	p.CurrencyRates = maps.Clone(p.CurrencyRates)
	for currency, value := range rates {
		entry, ok := p.CurrencyRates[currency]
		if !ok || !importRatePattern.MatchString(value) {
			return model.ImportPreview{}, importError("invalid_currency_rate", apperr.ErrValidation)
		}
		rate, valid := new(big.Rat).SetString(value)
		if !valid || rate.Sign() <= 0 {
			return model.ImportPreview{}, importError("invalid_currency_rate", apperr.ErrValidation)
		}
		entry.Rate, entry.Source = value, "manual"
		p.CurrencyRates[currency] = entry
	}
	kinds := make(map[string]model.AccountKind, len(p.Accounts))
	for _, a := range p.Accounts {
		kinds[a.SourceID] = a.Kind
	}
	p.ID = uuid.NewString()
	return s.prepare(ctx, p, kinds)
}

func prepareImportCurrency(ctx context.Context, repo importRepo, p *model.ImportPreview) error {
	p.Report.Transactions = slices.Clone(p.Report.Transactions)
	p.CurrencyRates = maps.Clone(p.CurrencyRates)
	if p.CurrencyRates == nil {
		p.CurrencyRates = map[string]model.ImportCurrencyRate{}
	}
	for code, entry := range p.CurrencyRates {
		entry.Transactions = 0
		p.CurrencyRates[code] = entry
	}
	var current map[string]string
	for i := range p.Report.Transactions {
		t := &p.Report.Transactions[i]
		if t.BaseAmountSource == "historical" || (t.BaseAmountSource == "" && t.DefaultCurrencyAmount != nil) {
			continue
		}
		if t.DefaultCurrency == "" {
			p.Report.Diagnostics = append(p.Report.Diagnostics, monefy.Diagnostic{Severity: monefy.Blocking, Code: "target_base_currency", Entity: "Transaction", SourceID: t.ID, Message: "Не определена поддерживаемая базовая валюта Monefy"})
			continue
		}
		entry, exists := p.CurrencyRates[t.Currency]
		if !exists {
			entry = model.ImportCurrencyRate{Currency: t.Currency, BaseCurrency: t.DefaultCurrency, Source: "current"}
			if current == nil {
				var err error
				current, err = repo.CurrencyRates(ctx)
				if err != nil {
					return err
				}
			}
			from, fromOK := new(big.Rat).SetString(current[t.Currency])
			to, toOK := new(big.Rat).SetString(current[t.DefaultCurrency])
			if fromOK && toOK && from.Sign() > 0 && to.Sign() > 0 {
				entry.Rate = new(big.Rat).Quo(to, from).FloatString(12)
			}
		}
		entry.Transactions++
		p.CurrencyRates[t.Currency] = entry
		t.DefaultCurrencyAmount = nil
		rate, valid := new(big.Rat).SetString(entry.Rate)
		if !valid || rate.Sign() <= 0 {
			p.Report.Diagnostics = append(p.Report.Diagnostics, monefy.Diagnostic{Severity: monefy.Blocking, Code: "target_missing_currency_rate", Entity: "Transaction", SourceID: t.ID, Message: "Нет текущего курса: укажите курс вручную в предпросмотре"})
			continue
		}
		amount := new(big.Int).Abs(big.NewInt(int64(t.Amount)))
		amount.Mul(amount, rate.Num()).Quo(amount, rate.Denom())
		if !amount.IsInt64() || amount.Sign() <= 0 || amount.Cmp(big.NewInt(99999999999999)) > 0 {
			p.Report.Diagnostics = append(p.Report.Diagnostics, monefy.Diagnostic{Severity: monefy.Blocking, Code: "target_base_amount_range", Entity: "Transaction", SourceID: t.ID, Message: "Сумма по выбранному курсу равна нулю или слишком велика"})
			continue
		}
		value := monefy.Amount(amount.Int64())
		t.DefaultCurrencyAmount, t.BaseAmountSource = &value, entry.Source
	}
	// Replace parser warnings with the exact fallback that will be persisted.
	diagnostics := p.Report.Diagnostics[:0]
	for _, diagnostic := range p.Report.Diagnostics {
		if diagnostic.Code != "missing_base_amount" {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	p.Report.Diagnostics = diagnostics
	return nil
}
