package service

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

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
	UpdateProfile(ctx context.Context, id string, patch model.UserProfilePatch, expectedHash string) (model.User, error)
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

// UpdateProfileReq contains only fields explicitly supplied by the caller.
type UpdateProfileReq struct {
	Username        *string
	Email           *string
	DefaultCurrency *string
	NewPassword     *string
	CurrentPassword string
}

func (s *UserService) UpdateProfile(ctx context.Context, id string, req UpdateProfileReq) (model.User, error) {
	patch := model.UserProfilePatch{}
	if req.Username == nil && req.Email == nil && req.DefaultCurrency == nil && req.NewPassword == nil {
		return model.User{}, fmt.Errorf("%w: укажите поля для изменения", apperr.ErrValidation)
	}
	if req.Username != nil {
		value := strings.TrimSpace(*req.Username)
		if value == "" || utf8.RuneCountInString(value) > 50 {
			return model.User{}, fmt.Errorf("%w: имя должно содержать от 1 до 50 символов", apperr.ErrValidation)
		}
		patch.Username = &value
	}
	if req.Email != nil {
		value := strings.ToLower(strings.TrimSpace(*req.Email))
		address, err := mail.ParseAddress(value)
		if err != nil || address.Address != value || len(value) > 255 {
			return model.User{}, fmt.Errorf("%w: укажите корректную почту", apperr.ErrValidation)
		}
		patch.Email = &value
	}
	if req.DefaultCurrency != nil {
		value := strings.ToUpper(strings.TrimSpace(*req.DefaultCurrency))
		if len(value) != 3 || strings.IndexFunc(value, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
			return model.User{}, fmt.Errorf("%w: defaultCurrency must be a 3-letter ISO code", apperr.ErrValidation)
		}
		patch.DefaultCurrency = &value
	}
	if req.NewPassword != nil && (len(*req.NewPassword) < 8 || len(*req.NewPassword) > 72) {
		return model.User{}, fmt.Errorf("%w: новый пароль должен содержать от 8 до 72 байт", apperr.ErrValidation)
	}
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return model.User{}, fmt.Errorf("get user: %w", err)
	}
	// An unchanged email must not overwrite a concurrent email change.
	if patch.Email != nil && *patch.Email == user.Email {
		patch.Email = nil
	}
	sensitive := req.NewPassword != nil || patch.Email != nil
	expectedHash := ""
	if sensitive {
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
			return model.User{}, fmt.Errorf("%w: неверный текущий пароль", apperr.ErrValidation)
		}
		expectedHash = user.PasswordHash
	}
	if req.NewPassword != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return model.User{}, fmt.Errorf("hash password: %w", err)
		}
		value := string(hash)
		patch.PasswordHash = &value
	}
	updated, err := s.repo.UpdateProfile(ctx, id, patch, expectedHash)
	if err != nil {
		return model.User{}, fmt.Errorf("update profile: %w", err)
	}
	return updated, nil
}
