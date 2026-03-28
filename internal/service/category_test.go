package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
)

// --- mock ---

type mockCategoryRepo struct {
	createFn          func(ctx context.Context, userID string, req model.CreateCategoryReq) (model.Category, error)
	getByIDFn         func(ctx context.Context, id, userID string) (model.Category, error)
	listByUserFn      func(ctx context.Context, userID string, catType model.CategoryType) ([]model.Category, error)
	updateFn          func(ctx context.Context, id, userID string, req model.UpdateCategoryReq) (model.Category, error)
	hasChildrenFn     func(ctx context.Context, id string) (bool, error)
	hasTransactionsFn func(ctx context.Context, id string) (bool, error)
	softDeleteFn      func(ctx context.Context, id, userID string) error
	hardDeleteFn      func(ctx context.Context, id, userID string) error
}

func (m *mockCategoryRepo) Create(ctx context.Context, userID string, req model.CreateCategoryReq) (model.Category, error) {
	return m.createFn(ctx, userID, req)
}
func (m *mockCategoryRepo) GetByID(ctx context.Context, id, userID string) (model.Category, error) {
	return m.getByIDFn(ctx, id, userID)
}
func (m *mockCategoryRepo) ListByUser(ctx context.Context, userID string, catType model.CategoryType) ([]model.Category, error) {
	return m.listByUserFn(ctx, userID, catType)
}
func (m *mockCategoryRepo) Update(ctx context.Context, id, userID string, req model.UpdateCategoryReq) (model.Category, error) {
	return m.updateFn(ctx, id, userID, req)
}
func (m *mockCategoryRepo) HasChildren(ctx context.Context, id string) (bool, error) {
	return m.hasChildrenFn(ctx, id)
}
func (m *mockCategoryRepo) HasTransactions(ctx context.Context, id string) (bool, error) {
	return m.hasTransactionsFn(ctx, id)
}
func (m *mockCategoryRepo) SoftDelete(ctx context.Context, id, userID string) error {
	return m.softDeleteFn(ctx, id, userID)
}
func (m *mockCategoryRepo) HardDelete(ctx context.Context, id, userID string) error {
	return m.hardDeleteFn(ctx, id, userID)
}

// --- helpers ---

func ptr(s string) *string { return &s }

func cat(id string, parentID *string, name string) model.Category {
	return model.Category{
		ID:       id,
		UserID:   "user1",
		ParentID: parentID,
		Name:     name,
		Type:     model.CategoryTypeExpense,
	}
}

func newSvc(repo categoryRepo) *CategoryService {
	return &CategoryService{repo: repo}
}

// --- buildTree tests ---

func TestBuildTree_Empty(t *testing.T) {
	result := buildTree(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty tree, got %d nodes", len(result))
	}
}

func TestBuildTree_SingleRoot(t *testing.T) {
	cats := []model.Category{cat("1", nil, "Food")}
	tree := buildTree(cats)
	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	if tree[0].Name != "Food" {
		t.Errorf("expected Food, got %s", tree[0].Name)
	}
	if len(tree[0].Children) != 0 {
		t.Errorf("expected no children")
	}
}

func TestBuildTree_OneLevel(t *testing.T) {
	cats := []model.Category{
		cat("1", nil, "Food"),
		cat("2", ptr("1"), "Restaurants"),
		cat("3", ptr("1"), "Groceries"),
	}
	tree := buildTree(cats)
	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	if len(tree[0].Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(tree[0].Children))
	}
}

func TestBuildTree_MultiLevel(t *testing.T) {
	// Food → Restaurants → FastFood
	cats := []model.Category{
		cat("1", nil, "Food"),
		cat("2", ptr("1"), "Restaurants"),
		cat("3", ptr("2"), "FastFood"),
	}
	tree := buildTree(cats)
	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("expected 1 child of root, got %d", len(tree[0].Children))
	}
	if len(tree[0].Children[0].Children) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(tree[0].Children[0].Children))
	}
	if tree[0].Children[0].Children[0].Name != "FastFood" {
		t.Errorf("expected FastFood at depth 2")
	}
}

func TestBuildTree_OrphanedNodeSurfacesAsRoot(t *testing.T) {
	// parent "1" is not in the list (soft-deleted) → child "2" should be a root
	cats := []model.Category{
		cat("2", ptr("1"), "Restaurants"),
	}
	tree := buildTree(cats)
	if len(tree) != 1 {
		t.Fatalf("expected orphan to surface as root, got %d roots", len(tree))
	}
	if tree[0].ID != "2" {
		t.Errorf("expected orphan node as root")
	}
}

func TestBuildTree_MultipleRoots(t *testing.T) {
	cats := []model.Category{
		cat("1", nil, "Food"),
		cat("2", nil, "Transport"),
		cat("3", ptr("1"), "Groceries"),
	}
	tree := buildTree(cats)
	if len(tree) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(tree))
	}
}

// --- CategoryService.Create tests ---

func TestCreate_Success(t *testing.T) {
	created := model.Category{ID: "new", Name: "Food", Type: model.CategoryTypeExpense, CreatedAt: time.Now()}
	svc := newSvc(&mockCategoryRepo{
		createFn: func(_ context.Context, _ string, _ model.CreateCategoryReq) (model.Category, error) {
			return created, nil
		},
	})
	got, err := svc.Create(context.Background(), "user1", model.CreateCategoryReq{
		Name: "Food",
		Type: model.CategoryTypeExpense,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "new" {
		t.Errorf("expected id=new, got %s", got.ID)
	}
}

func TestCreate_EmptyName(t *testing.T) {
	svc := newSvc(&mockCategoryRepo{})
	_, err := svc.Create(context.Background(), "user1", model.CreateCategoryReq{
		Name: "   ",
		Type: model.CategoryTypeExpense,
	})
	if !errors.Is(err, apperr.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestCreate_InvalidType(t *testing.T) {
	svc := newSvc(&mockCategoryRepo{})
	_, err := svc.Create(context.Background(), "user1", model.CreateCategoryReq{
		Name: "Food",
		Type: "unknown",
	})
	if !errors.Is(err, apperr.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestCreate_ParentTypeMismatch(t *testing.T) {
	parentID := "parent1"
	svc := newSvc(&mockCategoryRepo{
		getByIDFn: func(_ context.Context, _, _ string) (model.Category, error) {
			return model.Category{ID: parentID, Type: model.CategoryTypeIncome}, nil
		},
	})
	_, err := svc.Create(context.Background(), "user1", model.CreateCategoryReq{
		Name:     "Food",
		Type:     model.CategoryTypeExpense,
		ParentID: &parentID,
	})
	if !errors.Is(err, apperr.ErrValidation) {
		t.Errorf("expected ErrValidation for type mismatch, got %v", err)
	}
}

func TestCreate_ParentNotFound(t *testing.T) {
	parentID := "missing"
	svc := newSvc(&mockCategoryRepo{
		getByIDFn: func(_ context.Context, _, _ string) (model.Category, error) {
			return model.Category{}, apperr.ErrNotFound
		},
	})
	_, err := svc.Create(context.Background(), "user1", model.CreateCategoryReq{
		Name:     "Food",
		Type:     model.CategoryTypeExpense,
		ParentID: &parentID,
	})
	if !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCreate_NameTrimmed(t *testing.T) {
	var gotReq model.CreateCategoryReq
	svc := newSvc(&mockCategoryRepo{
		createFn: func(_ context.Context, _ string, req model.CreateCategoryReq) (model.Category, error) {
			gotReq = req
			return model.Category{Name: req.Name}, nil
		},
	})
	_, _ = svc.Create(context.Background(), "user1", model.CreateCategoryReq{
		Name: "  Food  ",
		Type: model.CategoryTypeExpense,
	})
	if gotReq.Name != "Food" {
		t.Errorf("expected trimmed name 'Food', got '%s'", gotReq.Name)
	}
}

// --- CategoryService.Update tests ---

func TestUpdate_EmptyName(t *testing.T) {
	svc := newSvc(&mockCategoryRepo{})
	empty := ""
	_, err := svc.Update(context.Background(), "user1", "id1", model.UpdateCategoryReq{Name: &empty})
	if !errors.Is(err, apperr.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestUpdate_NilNameAllowed(t *testing.T) {
	updated := model.Category{ID: "id1", Name: "Food"}
	svc := newSvc(&mockCategoryRepo{
		updateFn: func(_ context.Context, _, _ string, _ model.UpdateCategoryReq) (model.Category, error) {
			return updated, nil
		},
	})
	// name=nil means "don't change name" — should succeed
	_, err := svc.Update(context.Background(), "user1", "id1", model.UpdateCategoryReq{Name: nil})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- CategoryService.Delete tests ---

func TestDelete_HardDeleteWhenLeafNoTransactions(t *testing.T) {
	hardCalled := false
	svc := newSvc(&mockCategoryRepo{
		getByIDFn: func(_ context.Context, _, _ string) (model.Category, error) {
			return model.Category{ID: "id1"}, nil
		},
		hasChildrenFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
		hasTransactionsFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
		hardDeleteFn: func(_ context.Context, _, _ string) error {
			hardCalled = true
			return nil
		},
		softDeleteFn: func(_ context.Context, _, _ string) error {
			t.Error("soft delete should not be called")
			return nil
		},
	})
	if err := svc.Delete(context.Background(), "user1", "id1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hardCalled {
		t.Error("expected hard delete to be called")
	}
}

func TestDelete_SoftDeleteWhenHasChildren(t *testing.T) {
	softCalled := false
	svc := newSvc(&mockCategoryRepo{
		getByIDFn: func(_ context.Context, _, _ string) (model.Category, error) {
			return model.Category{ID: "id1"}, nil
		},
		hasChildrenFn:     func(_ context.Context, _ string) (bool, error) { return true, nil },
		hasTransactionsFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
		softDeleteFn: func(_ context.Context, _, _ string) error {
			softCalled = true
			return nil
		},
		hardDeleteFn: func(_ context.Context, _, _ string) error {
			t.Error("hard delete should not be called")
			return nil
		},
	})
	if err := svc.Delete(context.Background(), "user1", "id1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !softCalled {
		t.Error("expected soft delete to be called")
	}
}

func TestDelete_SoftDeleteWhenHasTransactions(t *testing.T) {
	softCalled := false
	svc := newSvc(&mockCategoryRepo{
		getByIDFn: func(_ context.Context, _, _ string) (model.Category, error) {
			return model.Category{ID: "id1"}, nil
		},
		hasChildrenFn:     func(_ context.Context, _ string) (bool, error) { return false, nil },
		hasTransactionsFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
		softDeleteFn: func(_ context.Context, _, _ string) error {
			softCalled = true
			return nil
		},
		hardDeleteFn: func(_ context.Context, _, _ string) error {
			t.Error("hard delete should not be called")
			return nil
		},
	})
	if err := svc.Delete(context.Background(), "user1", "id1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !softCalled {
		t.Error("expected soft delete to be called")
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc := newSvc(&mockCategoryRepo{
		getByIDFn: func(_ context.Context, _, _ string) (model.Category, error) {
			return model.Category{}, apperr.ErrNotFound
		},
	})
	err := svc.Delete(context.Background(), "user1", "missing")
	if !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- CategoryService.List tests ---

func TestList_ReturnsTree(t *testing.T) {
	svc := newSvc(&mockCategoryRepo{
		listByUserFn: func(_ context.Context, _ string, _ model.CategoryType) ([]model.Category, error) {
			return []model.Category{
				cat("1", nil, "Food"),
				cat("2", ptr("1"), "Restaurants"),
			}, nil
		},
	})
	tree, err := svc.List(context.Background(), "user1", model.CategoryTypeExpense)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("expected 1 root, got %d", len(tree))
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree[0].Children))
	}
}

func TestList_EmptyReturnsEmptySlice(t *testing.T) {
	svc := newSvc(&mockCategoryRepo{
		listByUserFn: func(_ context.Context, _ string, _ model.CategoryType) ([]model.Category, error) {
			return nil, nil
		},
	})
	tree, err := svc.List(context.Background(), "user1", model.CategoryTypeExpense)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tree == nil {
		t.Error("expected non-nil empty slice")
	}
}
