package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

type CategoryService struct {
	repo *repository.CategoryRepository
}

func NewCategoryService(repo *repository.CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

// CategoryNode is a category with its children for the tree response.
type CategoryNode struct {
	model.Category
	Children []CategoryNode `json:"children"`
}

func (s *CategoryService) Create(ctx context.Context, userID string, req model.CreateCategoryReq) (model.Category, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return model.Category{}, fmt.Errorf("name is required: %w", apperr.ErrValidation)
	}
	if req.Type != model.CategoryTypeExpense && req.Type != model.CategoryTypeIncome {
		return model.Category{}, fmt.Errorf("type must be expense or income: %w", apperr.ErrValidation)
	}
	if req.ParentID != nil {
		parent, err := s.repo.GetByID(ctx, *req.ParentID, userID)
		if err != nil {
			return model.Category{}, fmt.Errorf("parent category: %w", err)
		}
		if parent.Type != req.Type {
			return model.Category{}, fmt.Errorf("parent category type mismatch: %w", apperr.ErrValidation)
		}
	}
	return s.repo.Create(ctx, userID, req)
}

func (s *CategoryService) List(ctx context.Context, userID string, catType model.CategoryType) ([]CategoryNode, error) {
	cats, err := s.repo.ListByUser(ctx, userID, catType)
	if err != nil {
		return nil, err
	}
	return buildTree(cats), nil
}

func (s *CategoryService) Update(ctx context.Context, userID, id string, req model.UpdateCategoryReq) (model.Category, error) {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return model.Category{}, fmt.Errorf("name cannot be empty: %w", apperr.ErrValidation)
		}
		req.Name = &name
	}
	return s.repo.Update(ctx, id, userID, req)
}

func (s *CategoryService) Delete(ctx context.Context, userID, id string) error {
	// Verify ownership
	if _, err := s.repo.GetByID(ctx, id, userID); err != nil {
		return err
	}

	hasTransactions, err := s.repo.HasTransactions(ctx, id)
	if err != nil {
		return err
	}
	if hasTransactions {
		return s.repo.SoftDelete(ctx, id, userID)
	}
	return s.repo.HardDelete(ctx, id, userID)
}

// buildTree converts a flat list of categories into a nested tree.
func buildTree(cats []model.Category) []CategoryNode {
	byID := make(map[string]*CategoryNode, len(cats))
	for i := range cats {
		node := &CategoryNode{Category: cats[i]}
		byID[cats[i].ID] = node
	}

	var roots []CategoryNode
	for i := range cats {
		node := byID[cats[i].ID]
		if cats[i].ParentID == nil {
			roots = append(roots, *node)
			continue
		}
		if parent, ok := byID[*cats[i].ParentID]; ok {
			parent.Children = append(parent.Children, *node)
		} else {
			// orphaned (parent soft-deleted) — surface as root
			roots = append(roots, *node)
		}
	}
	return roots
}
