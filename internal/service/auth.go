package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
)

//go:generate mockgen -source=auth.go -destination=mocks/mock_auth_user_repo.go -package=mocks

type authUserRepo interface {
	GetByEmail(ctx context.Context, email string) (model.User, error)
	GetByID(ctx context.Context, id string) (model.User, error)
}

type AuthService struct {
	users     authUserRepo
	jwtSecret []byte
}

func NewAuthService(users authUserRepo, jwtSecret string) *AuthService {
	return &AuthService{users: users, jwtSecret: []byte(jwtSecret)}
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type Claims struct {
	UserID  string `json:"userId"`
	IsAdmin bool   `json:"isAdmin"`
	jwt.RegisteredClaims
}

func (s *AuthService) Login(ctx context.Context, email, password string) (model.User, TokenPair, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			return model.User{}, TokenPair{}, fmt.Errorf("invalid credentials: %w", apperr.ErrUnauthorized)
		}
		return model.User{}, TokenPair{}, fmt.Errorf("lookup user: %w", err)
	}
	if !u.IsActive {
		return model.User{}, TokenPair{}, fmt.Errorf("account is deactivated: %w", apperr.ErrUnauthorized)
	}
	if err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return model.User{}, TokenPair{}, fmt.Errorf("invalid credentials: %w", apperr.ErrUnauthorized)
	}
	tokens, err := s.issueTokens(u)
	if err != nil {
		return model.User{}, TokenPair{}, err
	}
	return u, tokens, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return TokenPair{}, fmt.Errorf("invalid refresh token: %w", apperr.ErrUnauthorized)
	}
	u, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			return TokenPair{}, fmt.Errorf("user not found: %w", apperr.ErrUnauthorized)
		}
		return TokenPair{}, fmt.Errorf("lookup user: %w", err)
	}
	return s.issueTokens(u)
}

func (s *AuthService) ValidateAccessToken(tokenStr string) (*Claims, error) {
	return s.parseToken(tokenStr)
}

func (s *AuthService) IssueTokens(u model.User) (TokenPair, error) {
	return s.issueTokens(u)
}

func (s *AuthService) issueTokens(u model.User) (TokenPair, error) {
	access, err := s.signToken(u, 15*time.Minute)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.signToken(u, 30*24*time.Hour)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

func (s *AuthService) signToken(u model.User, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID:  u.ID,
		IsAdmin: u.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func (s *AuthService) parseToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", apperr.ErrUnauthorized)
	}
	return claims, nil
}
