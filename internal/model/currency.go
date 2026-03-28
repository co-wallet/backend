package model

import "time"

type Currency struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Symbol   *string `json:"symbol,omitempty"`
	IsActive bool    `json:"isActive"`
}

type ExchangeRate struct {
	BaseCurrency  string    `json:"baseCurrency"`
	QuoteCurrency string    `json:"quoteCurrency"`
	Rate          float64   `json:"rate"`
	FetchedAt     time.Time `json:"fetchedAt"`
}

// CurrencyWithRate is returned by the API — active currency with rate relative to USD.
type CurrencyWithRate struct {
	Currency
	RateToUSD float64 `json:"rateToUsd"`
}
