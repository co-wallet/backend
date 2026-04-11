package invite

import (
	"time"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type InviteResponse struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Token     string     `json:"token"`
	CreatedBy string     `json:"createdBy"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
	ExpiresAt time.Time  `json:"expiresAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

func toInviteResponse(inv model.Invite) InviteResponse {
	return InviteResponse{
		ID:        inv.ID,
		Email:     inv.Email,
		Token:     inv.Token,
		CreatedBy: inv.CreatedBy,
		UsedAt:    inv.UsedAt,
		ExpiresAt: inv.ExpiresAt,
		CreatedAt: inv.CreatedAt,
	}
}

func toInviteResponses(items []model.Invite) []InviteResponse {
	out := make([]InviteResponse, len(items))
	for i, inv := range items {
		out[i] = toInviteResponse(inv)
	}
	return out
}

type CreateInviteResponse struct {
	Invite    InviteResponse `json:"invite"`
	InviteURL string         `json:"inviteUrl"`
}

type UserResponse struct {
	ID              string    `json:"id"`
	Username        string    `json:"username"`
	Email           string    `json:"email"`
	DefaultCurrency string    `json:"defaultCurrency"`
	IsAdmin         bool      `json:"isAdmin"`
	IsActive        bool      `json:"isActive"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func toUserResponse(u model.User) UserResponse {
	return UserResponse{
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

type TokenPairResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

func toTokenPairResponse(t service.TokenPair) TokenPairResponse {
	return TokenPairResponse{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken}
}

type AcceptInviteResponse struct {
	User   UserResponse      `json:"user"`
	Tokens TokenPairResponse `json:"tokens"`
}
