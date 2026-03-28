package service

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/co-wallet/backend/internal/model"
)

type adminRepo interface {
	ListUsers(ctx context.Context) ([]model.User, error)
	GetUser(ctx context.Context, id string) (*model.User, error)
	UpdateUser(ctx context.Context, id string, patch model.AdminUserPatch) error
	ListAllCurrencies(ctx context.Context) ([]model.CurrencyWithRate, error)
	CreateCurrency(ctx context.Context, c model.Currency) error
	UpdateCurrency(ctx context.Context, code string, patch model.CurrencyPatch) error
}

type AdminService struct {
	repo        adminRepo
	currencySvc *CurrencyService
}

func NewAdminService(repo adminRepo, currencySvc *CurrencyService) *AdminService {
	return &AdminService{repo: repo, currencySvc: currencySvc}
}

func (s *AdminService) ListUsers(ctx context.Context) ([]model.User, error) {
	return s.repo.ListUsers(ctx)
}

func (s *AdminService) GetUser(ctx context.Context, id string) (*model.User, error) {
	return s.repo.GetUser(ctx, id)
}

type AdminUpdateUserReq struct {
	IsActive    *bool   `json:"isActive"`
	IsAdmin     *bool   `json:"isAdmin"`
	NewPassword *string `json:"newPassword"`
}

func (s *AdminService) UpdateUser(ctx context.Context, id string, req AdminUpdateUserReq) error {
	patch := model.AdminUserPatch{
		IsActive: req.IsActive,
		IsAdmin:  req.IsAdmin,
	}
	if req.NewPassword != nil && *req.NewPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		h := string(hash)
		patch.PasswordHash = &h
	}
	return s.repo.UpdateUser(ctx, id, patch)
}

func (s *AdminService) ListAllCurrencies(ctx context.Context) ([]model.CurrencyWithRate, error) {
	return s.repo.ListAllCurrencies(ctx)
}

type CreateCurrencyReq struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Symbol   *string `json:"symbol"`
	IsActive bool    `json:"isActive"`
}

func (s *AdminService) CreateCurrency(ctx context.Context, req CreateCurrencyReq) error {
	return s.repo.CreateCurrency(ctx, model.Currency{
		Code:     req.Code,
		Name:     req.Name,
		Symbol:   req.Symbol,
		IsActive: req.IsActive,
	})
}

type UpdateCurrencyReq struct {
	Name     *string `json:"name"`
	Symbol   *string `json:"symbol"`
	IsActive *bool   `json:"isActive"`
}

func (s *AdminService) UpdateCurrency(ctx context.Context, code string, req UpdateCurrencyReq) error {
	return s.repo.UpdateCurrency(ctx, code, model.CurrencyPatch{
		Name:     req.Name,
		Symbol:   req.Symbol,
		IsActive: req.IsActive,
	})
}

func (s *AdminService) RefreshRates(ctx context.Context) error {
	return s.currencySvc.FetchAndStoreRates(ctx)
}
