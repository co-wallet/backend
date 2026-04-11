package invite

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

func TestInviteResponse_JSON(t *testing.T) {
	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	usedAt := now.Add(-time.Hour)
	inv := model.Invite{
		ID:        "i-1",
		Email:     "new@example.com",
		Token:     "tok-abc",
		CreatedBy: "admin-1",
		UsedAt:    &usedAt,
		ExpiresAt: now.Add(72 * time.Hour),
		CreatedAt: now,
	}

	raw, err := json.Marshal(toInviteResponse(inv))
	assert.NoError(t, err)

	var got map[string]any
	assert.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "i-1", got["id"])
	assert.Equal(t, "new@example.com", got["email"])
	assert.Equal(t, "tok-abc", got["token"])
	assert.Equal(t, "admin-1", got["createdBy"])
	assert.Contains(t, got, "usedAt")
	assert.Contains(t, got, "expiresAt")
	assert.Contains(t, got, "createdAt")
}

func TestInviteResponse_UsedAtOmittedWhenNil(t *testing.T) {
	inv := model.Invite{ID: "i-1", Email: "e@e", Token: "t"}
	raw, err := json.Marshal(toInviteResponse(inv))
	assert.NoError(t, err)

	var got map[string]any
	assert.NoError(t, json.Unmarshal(raw, &got))
	assert.NotContains(t, got, "usedAt")
}

func TestAcceptInviteResponse_JSON(t *testing.T) {
	u := model.User{
		ID:              "u-1",
		Username:        "bob",
		Email:           "b@c",
		PasswordHash:    "secret",
		DefaultCurrency: "USD",
		IsAdmin:         false,
		IsActive:        true,
	}
	resp := AcceptInviteResponse{
		User:   toUserResponse(u),
		Tokens: toTokenPairResponse(service.TokenPair{AccessToken: "a", RefreshToken: "r"}),
	}

	raw, err := json.Marshal(resp)
	assert.NoError(t, err)

	var got map[string]any
	assert.NoError(t, json.Unmarshal(raw, &got))

	user := got["user"].(map[string]any)
	assert.Equal(t, "u-1", user["id"])
	assert.Equal(t, "bob", user["username"])
	assert.NotContains(t, user, "passwordHash", "password hash must not leak")

	tokens := got["tokens"].(map[string]any)
	assert.Equal(t, "a", tokens["accessToken"])
	assert.Equal(t, "r", tokens["refreshToken"])
}
