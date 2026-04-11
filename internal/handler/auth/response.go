package auth

import (
	"time"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

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

type PublicUserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type TokenPairResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

type LoginResponse struct {
	User   UserResponse      `json:"user"`
	Tokens TokenPairResponse `json:"tokens"`
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

func toPublicUserResponse(u model.User) PublicUserResponse {
	return PublicUserResponse{ID: u.ID, Username: u.Username, Email: u.Email}
}

func toTokenPairResponse(t service.TokenPair) TokenPairResponse {
	return TokenPairResponse{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken}
}
