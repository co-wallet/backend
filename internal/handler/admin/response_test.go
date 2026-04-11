package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
)

func TestAdminUserResponse_JSON(t *testing.T) {
	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	u := model.User{
		ID:              "u-1",
		Username:        "alice",
		Email:           "a@b.c",
		PasswordHash:    "secret-hash",
		DefaultCurrency: "USD",
		IsAdmin:         true,
		IsActive:        true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	raw, err := json.Marshal(toAdminUserResponse(u))
	assert.NoError(t, err)

	var got map[string]any
	assert.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, "u-1", got["id"])
	assert.Equal(t, "alice", got["username"])
	assert.Equal(t, "a@b.c", got["email"])
	assert.Equal(t, "USD", got["defaultCurrency"])
	assert.Equal(t, true, got["isAdmin"])
	assert.Equal(t, true, got["isActive"])
	assert.Contains(t, got, "createdAt")
	assert.Contains(t, got, "updatedAt")
	assert.NotContains(t, got, "passwordHash", "password hash must not leak")
}

func TestCurrencyResponse_JSON(t *testing.T) {
	c := model.CurrencyWithRate{
		Currency: model.Currency{
			Code:     "EUR",
			Name:     "Euro",
			Symbol:   ptr.To("€"),
			IsActive: true,
		},
		RateToUSD: 1.08,
	}

	raw, err := json.Marshal(toCurrencyResponse(c))
	assert.NoError(t, err)

	var got map[string]any
	assert.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, "EUR", got["code"])
	assert.Equal(t, "Euro", got["name"])
	assert.Equal(t, "€", got["symbol"])
	assert.Equal(t, true, got["isActive"])
	assert.Equal(t, 1.08, got["rateToUsd"])
}
