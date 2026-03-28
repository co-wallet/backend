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

	hasChildren, err := s.repo.HasChildren(ctx, id)
	if err != nil {
		return err
	}
	hasTransactions, err := s.repo.HasTransactions(ctx, id)
	if err != nil {
		return err
	}
	if hasChildren || hasTransactions {
		return s.repo.SoftDelete(ctx, id, userID)
	}
	return s.repo.HardDelete(ctx, id, userID)
}

// buildTree converts a flat list of categories into a nested tree.
func buildTree(cats []model.Category) []CategoryNode {
	byID := make(map[string]model.Category, len(cats))
	childrenOf := make(map[string][]string, len(cats))

	for _, c := range cats {
		byID[c.ID] = c
		if c.ParentID != nil {
			childrenOf[*c.ParentID] = append(childrenOf[*c.ParentID], c.ID)
		}
	}

	var buildNode func(id string) CategoryNode
	buildNode = func(id string) CategoryNode {
		node := CategoryNode{Category: byID[id]}
		for _, childID := range childrenOf[id] {
			node.Children = append(node.Children, buildNode(childID))
		}
		return node
	}

	var roots []CategoryNode
	for _, c := range cats {
		if c.ParentID == nil {
			roots = append(roots, buildNode(c.ID))
		} else if _, parentExists := byID[*c.ParentID]; !parentExists {
			// orphaned (parent soft-deleted) — surface as root
			roots = append(roots, buildNode(c.ID))
		}
	}
	return roots
}
