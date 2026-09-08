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

func (s *CategoryServiceSuite) TestDelete_HardDeleteWithoutTransactions() {
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").Return(model.Category{ID: "id1"}, nil)
	s.repo.EXPECT().HasTransactions(gomock.Any(), "id1").Return(false, nil)
	s.repo.EXPECT().HardDelete(gomock.Any(), "id1", "user1").Return(nil)

	s.NoError(s.svc.Delete(context.Background(), "user1", "id1"))
}

func (s *CategoryServiceSuite) TestDelete_SoftDeleteWhenHasTransactions() {
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").Return(model.Category{ID: "id1"}, nil)
	s.repo.EXPECT().HasTransactions(gomock.Any(), "id1").Return(true, nil)
	s.repo.EXPECT().SoftDelete(gomock.Any(), "id1", "user1").Return(nil)

	s.NoError(s.svc.Delete(context.Background(), "user1", "id1"))
}

func (s *CategoryServiceSuite) TestDelete_NotFound() {
	s.repo.EXPECT().GetByID(gomock.Any(), "missing", "user1").Return(model.Category{}, apperr.ErrNotFound)

	err := s.svc.Delete(context.Background(), "user1", "missing")
	s.True(errors.Is(err, apperr.ErrNotFound))
}

func (s *CategoryServiceSuite) TestList() {
	for _, catType := range []model.CategoryType{model.CategoryTypeExpense, model.CategoryTypeIncome} {
		s.Run(string(catType), func() {
			categories := []model.Category{
				{ID: "1", Name: "Food", UserID: "user1", Type: catType},
				{ID: "2", Name: "Restaurants", UserID: "user1", Type: catType},
			}
			s.repo.EXPECT().ListByUser(gomock.Any(), "user1", catType).Return(categories, nil)
			got, err := s.svc.List(context.Background(), "user1", catType)
			s.NoError(err)
			s.Equal(categories, got)
		})
	}
}

func (s *CategoryServiceSuite) TestList_Empty() {
	s.repo.EXPECT().ListByUser(gomock.Any(), "user1", model.CategoryTypeExpense).Return(nil, nil)
	got, err := s.svc.List(context.Background(), "user1", model.CategoryTypeExpense)
	s.NoError(err)
	s.NotNil(got)
	s.Empty(got)
}

func (s *CategoryServiceSuite) TestList_RepositoryError() {
	repoErr := errors.New("database unavailable")
	s.repo.EXPECT().ListByUser(gomock.Any(), "user1", model.CategoryTypeExpense).Return(nil, repoErr)
	_, err := s.svc.List(context.Background(), "user1", model.CategoryTypeExpense)
	s.ErrorIs(err, repoErr)
}

func (s *CategoryServiceSuite) TestCreate_Conflict() {
	s.repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(model.Category{}, apperr.ErrConflict)
	_, err := s.svc.Create(context.Background(), "user1", model.CreateCategoryReq{Name: "Food", Type: model.CategoryTypeExpense})
	s.ErrorIs(err, apperr.ErrConflict)
}

func (s *CategoryServiceSuite) TestDelete_TransactionCheckError() {
	repoErr := errors.New("database unavailable")
	s.repo.EXPECT().GetByID(gomock.Any(), "id1", "user1").Return(model.Category{ID: "id1"}, nil)
	s.repo.EXPECT().HasTransactions(gomock.Any(), "id1").Return(false, repoErr)
	s.ErrorIs(s.svc.Delete(context.Background(), "user1", "id1"), repoErr)
}
