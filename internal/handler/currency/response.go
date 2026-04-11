package currencyhandler

import (
	"github.com/co-wallet/backend/internal/model"
)

type CurrencyResponse struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Symbol    *string `json:"symbol,omitempty"`
	IsActive  bool    `json:"isActive"`
	RateToUSD float64 `json:"rateToUsd"`
}

func toCurrencyResponse(c model.CurrencyWithRate) CurrencyResponse {
	return CurrencyResponse{
		Code:      c.Code,
		Name:      c.Name,
		Symbol:    c.Symbol,
		IsActive:  c.IsActive,
		RateToUSD: c.RateToUSD,
	}
}

func toCurrencyResponses(items []model.CurrencyWithRate) []CurrencyResponse {
	out := make([]CurrencyResponse, len(items))
	for i, c := range items {
		out[i] = toCurrencyResponse(c)
	}
	return out
}
