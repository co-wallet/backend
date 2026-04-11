package currencyhandler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
)

func TestCurrencyResponse_JSON(t *testing.T) {
	c := model.CurrencyWithRate{
		Currency: model.Currency{
			Code:     "USD",
			Name:     "US Dollar",
			Symbol:   ptr.To("$"),
			IsActive: true,
		},
		RateToUSD: 1.0,
	}

	raw, err := json.Marshal(toCurrencyResponse(c))
	assert.NoError(t, err)

	var got map[string]any
	assert.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, "USD", got["code"])
	assert.Equal(t, "US Dollar", got["name"])
	assert.Equal(t, "$", got["symbol"])
	assert.Equal(t, true, got["isActive"])
	assert.Equal(t, 1.0, got["rateToUsd"])
}

func TestCurrencyResponses_EmptySlice(t *testing.T) {
	got := toCurrencyResponses(nil)
	assert.Len(t, got, 0)
}
