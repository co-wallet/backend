package admin

import (
	"time"

	"github.com/co-wallet/backend/internal/model"
)

type AdminUserResponse struct {
	ID              string    `json:"id"`
	Username        string    `json:"username"`
	Email           string    `json:"email"`
	DefaultCurrency string    `json:"defaultCurrency"`
	IsAdmin         bool      `json:"isAdmin"`
	IsActive        bool      `json:"isActive"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func toAdminUserResponse(u model.User) AdminUserResponse {
	return AdminUserResponse{
		ID:              u.ID,
		Username:        u.Username,
		Email:           u.Email,
		DefaultCurrency: u.DefaultCurrency,
		IsAdmin:         u.IsAdmin,
		IsActive:        u.IsActive,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
	}
}

func toAdminUserResponses(users []model.User) []AdminUserResponse {
	out := make([]AdminUserResponse, len(users))
	for i, u := range users {
		out[i] = toAdminUserResponse(u)
	}
	return out
}

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
