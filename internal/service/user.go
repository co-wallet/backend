package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
)

//go:generate mockgen -source=user.go -destination=mocks/mock_user_repo.go -package=mocks

type UserRepo interface {
	GetByID(ctx context.Context, id string) (model.User, error)
	GetByEmail(ctx context.Context, email string) (model.User, error)
	GetByUsername(ctx context.Context, username string) (model.User, error)
	ListActive(ctx context.Context) ([]model.User, error)
	UpdateCurrency(ctx context.Context, id, currency string) error
}

type UserService struct {
	repo UserRepo
}

func NewUserService(repo UserRepo) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) GetByID(ctx context.Context, id string) (model.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *UserService) GetByUsername(ctx context.Context, username string) (model.User, error) {
	return s.repo.GetByUsername(ctx, username)
}

func (s *UserService) ListActive(ctx context.Context) ([]model.User, error) {
	users, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active users: %w", err)
	}
	if users == nil {
		users = []model.User{}
	}
	return users, nil
}

func (s *UserService) UpdateCurrency(ctx context.Context, id, currency string) (model.User, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return model.User{}, fmt.Errorf("%w: defaultCurrency must be a 3-letter ISO code", apperr.ErrValidation)
	}
	if err := s.repo.UpdateCurrency(ctx, id, currency); err != nil {
		return model.User{}, fmt.Errorf("update currency: %w", err)
	}
	return s.repo.GetByID(ctx, id)
}
