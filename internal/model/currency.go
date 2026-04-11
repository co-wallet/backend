package model

import "time"

type Currency struct {
	Code     string
	Name     string
	Symbol   *string
	IsActive bool
}

type ExchangeRate struct {
	BaseCurrency  string
	QuoteCurrency string
	Rate          float64
	FetchedAt     time.Time
}

type CurrencyPatch struct {
	Name     *string
	Symbol   *string
	IsActive *bool
}

// CurrencyWithRate is returned by services — active currency with rate relative to USD.
type CurrencyWithRate struct {
	Currency
	RateToUSD float64
}
