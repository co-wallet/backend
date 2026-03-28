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
	HasChildren(ctx context.Context, id string) (bool, error)
	HasTransactions(ctx context.Context, id string) (bool, error)
	SoftDelete(ctx context.Context, id, userID string) error
	HardDelete(ctx context.Context, id, userID string) error
}

type CategoryService struct {
	repo CategoryRepo
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
	return s.repo.Create(ctx, model.Category{
		UserID:   userID,
		ParentID: req.ParentID,
		Name:     req.Name,
		Type:     req.Type,
		Icon:     req.Icon,
	})
}

func (s *CategoryService) List(ctx context.Context, userID string, catType model.CategoryType) ([]CategoryNode, error) {
	cats, err := s.repo.ListByUser(ctx, userID, catType)
	if err != nil {
		return nil, err
	}
	tree := buildTree(cats)
	if tree == nil {
		tree = []CategoryNode{}
	}
	return tree, nil
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
