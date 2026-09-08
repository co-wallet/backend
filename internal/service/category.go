package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

// CategoryRepo is the repository interface consumed by CategoryService.
//
//go:generate mockgen -destination=mocks/mock_category_repo.go -package=mocks github.com/co-wallet/backend/internal/service CategoryRepo
type CategoryRepo interface {
	Create(ctx context.Context, c model.Category) (model.Category, error)
	GetByID(ctx context.Context, id, userID string) (model.Category, error)
	ListByUser(ctx context.Context, userID string, catType model.CategoryType) ([]model.Category, error)
	Update(ctx context.Context, c model.Category) (model.Category, error)
	HasTransactions(ctx context.Context, id string) (bool, error)
	SetHidden(ctx context.Context, id, userID string, hidden bool) error
	HardDelete(ctx context.Context, id, userID string) error
}

type CategoryService struct {
	repo CategoryRepo
}

func NewCategoryService(repo *repository.CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

func (s *CategoryService) Create(ctx context.Context, userID string, req model.CreateCategoryReq) (model.Category, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return model.Category{}, fmt.Errorf("name is required: %w", apperr.ErrValidation)
	}
	if !req.Type.IsValid() {
		return model.Category{}, fmt.Errorf("type must be expense or income: %w", apperr.ErrValidation)
	}
	return s.repo.Create(ctx, model.Category{
		UserID: userID,
		Name:   req.Name,
		Type:   req.Type,
		Icon:   req.Icon,
	})
}

func (s *CategoryService) List(ctx context.Context, userID string, catType model.CategoryType) ([]model.Category, error) {
	cats, err := s.repo.ListByUser(ctx, userID, catType)
	if err != nil {
		return nil, err
	}
	if cats == nil {
		cats = []model.Category{}
	}
	return cats, nil
}

func (s *CategoryService) Update(ctx context.Context, userID, id string, req model.UpdateCategoryReq) (model.Category, error) {
	existing, err := s.repo.GetByID(ctx, id, userID)
	if err != nil {
		return model.Category{}, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return model.Category{}, fmt.Errorf("name cannot be empty: %w", apperr.ErrValidation)
		}
		existing.Name = name
	}
	if req.Icon != nil {
		existing.Icon = req.Icon
	}
	return s.repo.Update(ctx, existing)
}

func (s *CategoryService) Delete(ctx context.Context, userID, id string) error {
	// Verify the shared entry exists.
	if _, err := s.repo.GetByID(ctx, id, userID); err != nil {
		return err
	}

	hasTransactions, err := s.repo.HasTransactions(ctx, id)
	if err != nil {
		return err
	}
	if hasTransactions {
		return fmt.Errorf("category has linked transactions: %w", apperr.ErrConflict)
	}
	return s.repo.HardDelete(ctx, id, userID)
}

func (s *CategoryService) SetHidden(ctx context.Context, userID, id string, hidden bool) error {
	if _, err := s.repo.GetByID(ctx, id, userID); err != nil {
		return err
	}
	return s.repo.SetHidden(ctx, id, userID, hidden)
}
