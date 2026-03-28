package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service/mocks"
)

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

// --- suite ---

type CategoryServiceSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	repo *mocks.MockCategoryRepo
	svc  *CategoryService
}

func (s *CategoryServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockCategoryRepo(s.ctrl)
	s.svc = &CategoryService{repo: s.repo}
}

func TestCategoryServiceSuite(t *testing.T) {
	suite.Run(t, new(CategoryServiceSuite))
}

// --- Create ---

func (s *CategoryServiceSuite) TestCreate_Success() {
	req := model.CreateCategoryReq{Name: "Food", Type: model.CategoryTypeExpense}
	s.repo.EXPECT().
		Create(gomock.Any(), model.Category{UserID: "user1", Name: "Food", Type: model.CategoryTypeExpense}).
		Return(model.Category{ID: "new", Name: "Food"}, nil)

	got, err := s.svc.Create(context.Background(), "user1", req)
	s.NoError(err)
	s.Equal("new", got.ID)
}

func (s *CategoryServiceSuite) TestCreate_EmptyName() {
	_, err := s.svc.Create(context.Background(), "user1", model.CreateCategoryReq{Name: "   ", Type: model.CategoryTypeExpense})
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *CategoryServiceSuite) TestCreate_InvalidType() {
	_, err := s.svc.Create(context.Background(), "user1", model.CreateCategoryReq{Name: "Food", Type: "unknown"})
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *CategoryServiceSuite) TestCreate_NameTrimmed() {
	s.repo.EXPECT().
		Create(gomock.Any(), gomock.AssignableToTypeOf(model.Category{})).
		DoAndReturn(func(_ context.Context, c model.Category) (model.Category, error) {
			s.Equal("Food", c.Name)
			return c, nil
		})

	_, _ = s.svc.Create(context.Background(), "user1", model.CreateCategoryReq{Name: "  Food  ", Type: model.CategoryTypeExpense})
}

func (s *CategoryServiceSuite) TestCreate_ParentTypeMismatch() {
	parentID := "parent1"
	s.repo.EXPECT().
		GetByID(gomock.Any(), parentID, "user1").
		Return(model.Category{ID: parentID, Type: model.CategoryTypeIncome}, nil)

	_, err := s.svc.Create(context.Background(), "user1", model.CreateCategoryReq{
		Name: "Food", Type: model.CategoryTypeExpense, ParentID: &parentID,
	})
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *CategoryServiceSuite) TestCreate_ParentNotFound() {
	parentID := "missing"
	s.repo.EXPECT().
		GetByID(gomock.Any(), parentID, "user1").
		Return(model.Category{}, apperr.ErrNotFound)

	_, err := s.svc.Create(context.Background(), "user1", model.CreateCategoryReq{
		Name: "Food", Type: model.CategoryTypeExpense, ParentID: &parentID,
	})
	s.True(errors.Is(err, apperr.ErrNotFound))
}

// --- Update ---

func (s *CategoryServiceSuite) TestUpdate_Success() {
	existing := model.Category{ID: "id1", UserID: "user1", Name: "Old", Icon: nil, Type: model.CategoryTypeExpense}
	newName := "New"
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").Return(existing, nil)
	s.repo.EXPECT().
		Update(gomock.Any(), model.Category{ID: "id1", UserID: "user1", Name: "New", Icon: nil, Type: model.CategoryTypeExpense}).
		Return(model.Category{ID: "id1", Name: "New"}, nil)

	got, err := s.svc.Update(context.Background(), "user1", "id1", model.UpdateCategoryReq{Name: &newName})
	s.NoError(err)
	s.Equal("New", got.Name)
}

func (s *CategoryServiceSuite) TestUpdate_EmptyName() {
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").
		Return(model.Category{ID: "id1", Name: "Food"}, nil)

	empty := ""
	_, err := s.svc.Update(context.Background(), "user1", "id1", model.UpdateCategoryReq{Name: &empty})
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *CategoryServiceSuite) TestUpdate_NilNameKeepsExisting() {
	existing := model.Category{ID: "id1", UserID: "user1", Name: "Food"}
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").Return(existing, nil)
	s.repo.EXPECT().Update(gomock.Any(), existing).Return(existing, nil)

	_, err := s.svc.Update(context.Background(), "user1", "id1", model.UpdateCategoryReq{Name: nil})
	s.NoError(err)
}

func (s *CategoryServiceSuite) TestUpdate_NotFound() {
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").Return(model.Category{}, apperr.ErrNotFound)

	_, err := s.svc.Update(context.Background(), "user1", "id1", model.UpdateCategoryReq{})
	s.True(errors.Is(err, apperr.ErrNotFound))
}

// --- Delete ---

func (s *CategoryServiceSuite) TestDelete_HardDeleteForLeaf() {
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").Return(model.Category{ID: "id1"}, nil)
	s.repo.EXPECT().HasChildren(gomock.Any(), "id1").Return(false, nil)
	s.repo.EXPECT().HasTransactions(gomock.Any(), "id1").Return(false, nil)
	s.repo.EXPECT().HardDelete(gomock.Any(), "id1", "user1").Return(nil)

	s.NoError(s.svc.Delete(context.Background(), "user1", "id1"))
}

func (s *CategoryServiceSuite) TestDelete_SoftDeleteWhenHasChildren() {
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").Return(model.Category{ID: "id1"}, nil)
	s.repo.EXPECT().HasChildren(gomock.Any(), "id1").Return(true, nil)
	s.repo.EXPECT().HasTransactions(gomock.Any(), "id1").Return(false, nil)
	s.repo.EXPECT().SoftDelete(gomock.Any(), "id1", "user1").Return(nil)

	s.NoError(s.svc.Delete(context.Background(), "user1", "id1"))
}

func (s *CategoryServiceSuite) TestDelete_SoftDeleteWhenHasTransactions() {
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").Return(model.Category{ID: "id1"}, nil)
	s.repo.EXPECT().HasChildren(gomock.Any(), "id1").Return(false, nil)
	s.repo.EXPECT().HasTransactions(gomock.Any(), "id1").Return(true, nil)
	s.repo.EXPECT().SoftDelete(gomock.Any(), "id1", "user1").Return(nil)

	s.NoError(s.svc.Delete(context.Background(), "user1", "id1"))
}

func (s *CategoryServiceSuite) TestDelete_NotFound() {
	s.repo.EXPECT().GetByID(gomock.Any(), "missing", "user1").Return(model.Category{}, apperr.ErrNotFound)

	err := s.svc.Delete(context.Background(), "user1", "missing")
	s.True(errors.Is(err, apperr.ErrNotFound))
}

// --- List (table-driven, tests buildTree through public API) ---

func (s *CategoryServiceSuite) TestList() {
	tests := []struct {
		name       string
		repoResult []model.Category
		wantRoots  int
		wantChild  string // expected first child name of first root, "" to skip
	}{
		{
			name:       "empty returns empty slice",
			repoResult: nil,
			wantRoots:  0,
		},
		{
			name:       "single root, no children",
			repoResult: []model.Category{cat("1", nil, "Food")},
			wantRoots:  1,
		},
		{
			name: "root with one child",
			repoResult: []model.Category{
				cat("1", nil, "Food"),
				cat("2", ptr("1"), "Restaurants"),
			},
			wantRoots: 1,
			wantChild: "Restaurants",
		},
		{
			name: "multiple roots",
			repoResult: []model.Category{
				cat("1", nil, "Food"),
				cat("2", nil, "Transport"),
			},
			wantRoots: 2,
		},
		{
			name: "orphaned node surfaces as root",
			repoResult: []model.Category{
				cat("2", ptr("missing"), "Restaurants"),
			},
			wantRoots: 1,
		},
		{
			name: "three levels deep",
			repoResult: []model.Category{
				cat("1", nil, "Food"),
				cat("2", ptr("1"), "Restaurants"),
				cat("3", ptr("2"), "FastFood"),
			},
			wantRoots: 1,
			wantChild: "Restaurants",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.repo.EXPECT().
				ListByUser(gomock.Any(), "user1", model.CategoryTypeExpense).
				Return(tt.repoResult, nil)

			tree, err := s.svc.List(context.Background(), "user1", model.CategoryTypeExpense)
			s.NoError(err)
			s.NotNil(tree)
			s.Len(tree, tt.wantRoots)

			if tt.wantChild != "" && len(tree) > 0 {
				s.Len(tree[0].Children, 1)
				s.Equal(tt.wantChild, tree[0].Children[0].Name)
			}
		})
	}
}
