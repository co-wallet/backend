package monefy

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"time"
)

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

func (r *Report) diagnostic(level Severity, code, entity, id, message string) {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{level, code, entity, id, message})
}

func normalize(tables map[string][]record, opts Options, out *Report) error {
	allowed := make(map[string]bool)
	for _, code := range opts.SupportedCurrencies {
		allowed[code] = true
	}
	currencies := make(map[int64]Currency)
	codes := make(map[string]bool)
	for _, row := range tables["Currency"] {
		c := Currency{ID: row.integer("Id"), Code: row.text("AlphabeticCode", false), Name: row.text("Name", false), Symbol: row.text("Symbol", true), MinorUnits: row.integer("MinorUnits"), IsBase: row.flag("IsBase")}
		if !currencyCode.MatchString(c.Code) || codes[c.Code] || c.ID <= 0 || row.integer("NumericCode") != c.ID || c.MinorUnits < 0 || c.MinorUnits > 3 {
			row.fail("Currency")
		}
		if row.err != nil {
			return row.err
		}
		currencies[c.ID], codes[c.Code] = c, true
		out.Currencies = append(out.Currencies, c)
	}
	accounts, deletedAccounts := make(map[string]Account), make(map[string]bool)
	for _, row := range tables["Account"] {
		id := row.text("Id", false)
		deleted := row.optionalDate("DeletedOn") != nil
		if row.err != nil {
			return row.err
		}
		if deleted {
			deletedAccounts[id] = true
			out.Deleted["Account"]++
			continue
		}
		a := Account{ID: id, Name: row.text("Name", false), Icon: row.integer("Icon"), CurrencyID: row.integer("CurrencyId"), CreatedAt: row.date("CreatedOn"), InitialBalance: Amount(row.integer("InitialBalanceCents")), IncludedInTotal: row.flag("IsIncludedInTotalBalance"), DisabledAt: row.optionalDate("DisabledOn")}
		if row.err != nil {
			return row.err
		}
		c, ok := currencies[a.CurrencyID]
		a.Currency = c.Code
		if !ok || !allowed[c.Code] {
			out.diagnostic(Blocking, "unknown_currency", "Account", id, "Валюта отсутствует в источнике или не поддерживается сервером")
		}
		if a.DisabledAt != nil {
			out.diagnostic(Warning, "disabled_account", "Account", id, "Счёт отключён в Monefy; его история сохранена")
		}
		accounts[id] = a
		out.Accounts = append(out.Accounts, a)
	}
	categories, deletedCategories := make(map[string]Category), make(map[string]bool)
	for _, row := range tables["Category"] {
		id := row.text("Id", false)
		deleted := row.optionalDate("DeletedOn") != nil
		if row.err != nil {
			return row.err
		}
		if deleted {
			deletedCategories[id] = true
			out.Deleted["Category"]++
			continue
		}
		c := Category{ID: id, Name: row.text("Name", false), Icon: row.integer("Icon"), DisabledAt: row.optionalDate("DisabledOn")}
		switch row.integer("CategoryType") {
		case 0:
			c.Type = "income"
		case 1:
			c.Type = "expense"
		default:
			row.fail("CategoryType")
		}
		if row.err != nil {
			return row.err
		}
		categories[id] = c
		out.Categories = append(out.Categories, c)
	}
	for _, row := range tables["CurrencyRate"] {
		id := row.text("Id", false)
		deleted := row.optionalDate("DeletedOn") != nil
		if row.err != nil {
			return row.err
		}
		if deleted {
			out.Deleted["CurrencyRate"]++
			continue
		}
		rate := Rate{ID: id, FromCurrencyID: row.integer("CurrencyFromId"), ToCurrencyID: row.integer("CurrencyToId"), Millionths: row.integer("RateCents"), Date: row.date("RateDate"), CreatedAt: row.date("CreatedOn")}
		if rate.Millionths <= 0 {
			row.fail("RateCents")
		}
		if row.err != nil {
			return row.err
		}
		_, fromOK := currencies[rate.FromCurrencyID]
		_, toOK := currencies[rate.ToCurrencyID]
		if !fromOK || !toOK || rate.FromCurrencyID == rate.ToCurrencyID {
			out.diagnostic(Blocking, "invalid_reference", "CurrencyRate", id, "Некорректная пара валют курса")
		}
		out.Rates = append(out.Rates, rate)
	}
	rates := indexRates(out.Rates)
	var baseCurrency Currency
	baseCount := 0
	for _, currency := range out.Currencies {
		if currency.IsBase {
			baseCurrency = currency
			baseCount++
		}
	}
	for _, row := range tables["Transaction"] {
		id := row.text("Id", false)
		deleted := row.optionalDate("DeletedOn") != nil
		if row.err != nil {
			return row.err
		}
		if deleted {
			out.Deleted["Transaction"]++
			continue
		}
		t := Transaction{ID: id, AccountID: row.text("AccountId", false), CategoryID: row.text("CategoryId", false), Amount: Amount(row.integer("AmountCents")), CreatedAt: row.date("CreatedOn"), Note: row.text("Note", true)}
		if t.Amount < 0 {
			row.fail("AmountCents")
		}
		if row.text("ScheduleId", true) != "" {
			out.diagnostic(Blocking, "unsupported_schedule", "Transaction", id, "Операция связана с расписанием; перенос расписаний не поддерживается")
		}
		if row.err != nil {
			return row.err
		}
		a, aOK := accounts[t.AccountID]
		c, cOK := categories[t.CategoryID]
		missing := (!aOK && !deletedAccounts[t.AccountID]) || (!cOK && !deletedCategories[t.CategoryID])
		if missing {
			out.diagnostic(Blocking, "invalid_reference", "Transaction", id, "Счёт или категория операции не найдены в источнике")
			continue
		}
		if !aOK || !cOK {
			exclude(out, "Transaction", id)
			continue
		}
		t.Type, t.Currency = c.Type, a.Currency
		if baseCount == 1 && allowed[baseCurrency.Code] {
			t.DefaultCurrency = baseCurrency.Code
			t.DefaultCurrencyAmount = historicalAmount(rates, t.Amount, a.CurrencyID, baseCurrency.ID, t.CreatedAt)
			if t.DefaultCurrencyAmount != nil {
				t.BaseAmountSource = "historical"
			}
		}
		if t.DefaultCurrencyAmount == nil {
			out.diagnostic(Warning, "missing_base_amount", "Transaction", id, "Нет однозначной базовой валюты или исторического курса: сумма в базовой валюте не заполнена; текущий курс не подставляется")
		}
		if c.Type == "expense" {
			t.Amount = -t.Amount
		}
		out.Transactions = append(out.Transactions, t)
	}
	for _, row := range tables["Transfer"] {
		id := row.text("Id", false)
		deleted := row.optionalDate("DeletedOn") != nil
		if row.err != nil {
			return row.err
		}
		if deleted {
			out.Deleted["Transfer"]++
			continue
		}
		t := Transfer{ID: id, FromAccountID: row.text("AccountFromId", false), ToAccountID: row.text("AccountToId", false), FromAmount: Amount(row.integer("AmountCents")), CreatedAt: row.date("CreatedOn"), Note: row.text("Note", true)}
		if t.FromAmount <= 0 {
			row.fail("AmountCents")
		}
		if t.FromAccountID == t.ToAccountID {
			row.fail("AccountToId")
		}
		if row.err != nil {
			return row.err
		}
		a, aOK := accounts[t.FromAccountID]
		b, bOK := accounts[t.ToAccountID]
		missing := (!aOK && !deletedAccounts[t.FromAccountID]) || (!bOK && !deletedAccounts[t.ToAccountID])
		if missing {
			out.diagnostic(Blocking, "invalid_reference", "Transfer", id, "Счёт перевода не найден в источнике")
			continue
		}
		if !aOK || !bOK {
			exclude(out, "Transfer", id)
			continue
		}
		t.FromCurrency, t.ToCurrency = a.Currency, b.Currency
		if a.CurrencyID == b.CurrencyID {
			amount := t.FromAmount
			t.ToAmount = &amount
		} else {
			resolveTransfer(out, rates, &t, a.CurrencyID, b.CurrencyID)
		}
		out.Transfers = append(out.Transfers, t)
	}
	for _, row := range tables["Schedule"] {
		id := row.text("Id", false)
		deleted := row.optionalDate("DeletedOn") != nil
		if row.err != nil {
			return row.err
		}
		if deleted {
			out.Deleted["Schedule"]++
			continue
		}
		out.diagnostic(Blocking, "unsupported_schedule", "Schedule", id, "В источнике есть расписание; его перенос не поддерживается")
	}
	for _, row := range tables["Setting"] {
		id := row.text("Id", false)
		if row.err != nil {
			return row.err
		}
		out.diagnostic(Blocking, "unsupported_setting", "Setting", id, "В источнике есть неподдерживаемая настройка")
	}
	return nil
}

func exclude(out *Report, entity, id string) {
	reason := "Живая запись ссылается на удалённый счёт или категорию; исключение требует подтверждения"
	out.Exclusions = append(out.Exclusions, Exclusion{entity, id, reason})
	out.diagnostic(Confirmation, "deleted_reference", entity, id, reason)
}

type ratePair struct{ from, to int64 }
type rateChoice struct {
	rate      Rate
	ambiguous bool
}

// Collapse revisions once, then perform logarithmic lookups per transfer.
func indexRates(rates []Rate) map[ratePair][]rateChoice {
	byDate := make(map[ratePair]map[time.Time]rateChoice)
	for _, rate := range rates {
		key := ratePair{rate.FromCurrencyID, rate.ToCurrencyID}
		if byDate[key] == nil {
			byDate[key] = make(map[time.Time]rateChoice)
		}
		choice, ok := byDate[key][rate.Date]
		if !ok || rate.CreatedAt.After(choice.rate.CreatedAt) {
			choice = rateChoice{rate: rate}
		} else if rate.CreatedAt.Equal(choice.rate.CreatedAt) && rate.Millionths != choice.rate.Millionths {
			choice.ambiguous = true
		}
		byDate[key][rate.Date] = choice
	}
	index := make(map[ratePair][]rateChoice)
	for key, dates := range byDate {
		for _, choice := range dates {
			index[key] = append(index[key], choice)
		}
		sort.Slice(index[key], func(i, j int) bool { return index[key][i].rate.Date.Before(index[key][j].rate.Date) })
	}
	return index
}

func resolveTransfer(out *Report, index map[ratePair][]rateChoice, transfer *Transfer, fromID, toID int64) {
	rates := index[ratePair{fromID, toID}]
	i := sort.Search(len(rates), func(i int) bool { return rates[i].rate.Date.After(transfer.CreatedAt) }) - 1
	if i < 0 || rates[i].ambiguous {
		out.diagnostic(Blocking, "ambiguous_rate", "Transfer", transfer.ID, "Нет однозначного прямого исторического курса на дату перевода; обратные и сетевые курсы не используются")
		return
	}
	selected := rates[i].rate
	// Monefy truncates the positive integer product to thousandths (not nearest ISO cents).
	product := new(big.Int).Mul(big.NewInt(int64(transfer.FromAmount)), big.NewInt(selected.Millionths))
	product.Quo(product, big.NewInt(1000000))
	if !product.IsInt64() || product.Sign() <= 0 {
		out.diagnostic(Blocking, "amount_range", "Transfer", transfer.ID, fmt.Sprintf("Сумма зачисления по курсу %s равна нулю или превышает int64", selected.ID))
		return
	}
	amount := Amount(product.Int64())
	transfer.ToAmount, transfer.RateID = &amount, selected.ID
}

// historicalAmount prefers the direct source rate, then the inverse pair.
// Future rates and conflicting revisions cannot establish a historical value.
// Keep integer thousandths and truncate only once, as for Monefy transfers.
func historicalAmount(index map[ratePair][]rateChoice, amount Amount, fromID, toID int64, date time.Time) *Amount {
	if fromID == toID {
		return &amount
	}
	for _, inverse := range []bool{false, true} {
		pair := ratePair{fromID, toID}
		if inverse {
			pair = ratePair{toID, fromID}
		}
		rates := index[pair]
		i := sort.Search(len(rates), func(i int) bool { return rates[i].rate.Date.After(date) }) - 1
		if i < 0 {
			continue
		}
		if rates[i].ambiguous {
			return nil
		}
		numerator, denominator := rates[i].rate.Millionths, int64(1000000)
		if inverse {
			numerator, denominator = denominator, numerator
		}
		product := new(big.Int).Mul(big.NewInt(int64(amount)), big.NewInt(numerator))
		product.Quo(product, big.NewInt(denominator))
		if !product.IsInt64() || product.Sign() <= 0 {
			return nil
		}
		converted := Amount(product.Int64())
		return &converted
	}
	return nil
}
