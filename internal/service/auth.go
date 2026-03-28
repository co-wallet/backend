package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

type AuthService struct {
	users     *repository.UserRepository
	jwtSecret []byte
}

func NewAuthService(users *repository.UserRepository, jwtSecret string) *AuthService {
	return &AuthService{users: users, jwtSecret: []byte(jwtSecret)}
}

type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

type Claims struct {
	UserID  string `json:"userId"`
	IsAdmin bool   `json:"isAdmin"`
	jwt.RegisteredClaims
}

func (s *AuthService) Register(ctx context.Context, username, email, password string) (*model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u := &model.User{
		Username:        username,
		Email:           email,
		PasswordHash:    string(hash),
		DefaultCurrency: "RUB",
		IsActive:        true,
	}
	if err = s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*model.User, *TokenPair, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, nil, errors.New("invalid credentials")
	}
	if !u.IsActive {
		return nil, nil, errors.New("account is deactivated")
	}
	if err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, nil, errors.New("invalid credentials")
	}
	tokens, err := s.issueTokens(u)
	if err != nil {
		return nil, nil, err
	}
	return u, tokens, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.parseToken(refreshToken)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}
	u, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}
	return s.issueTokens(u)
}

func (s *AuthService) ValidateAccessToken(tokenStr string) (*Claims, error) {
	return s.parseToken(tokenStr)
}

func (s *AuthService) IssueTokens(u *model.User) (*TokenPair, error) {
	return s.issueTokens(u)
}

func (s *AuthService) issueTokens(u *model.User) (*TokenPair, error) {
	access, err := s.signToken(u, 15*time.Minute)
	if err != nil {
		return nil, err
	}
	refresh, err := s.signToken(u, 30*24*time.Hour)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

func (s *AuthService) signToken(u *model.User, ttl time.Duration) (string, error) {
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
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
