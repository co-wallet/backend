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

type TagServiceSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	repo *mocks.MockTagRepo
	svc  *TagService
}

func (s *TagServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockTagRepo(s.ctrl)
	s.svc = &TagService{repo: s.repo}
}

func (s *TagServiceSuite) TearDownTest() {
	s.ctrl.Finish()
}

func TestTagServiceSuite(t *testing.T) {
	suite.Run(t, new(TagServiceSuite))
}

func (s *TagServiceSuite) TestList_NoQuery_ReturnsAll() {
	ctx := context.Background()
	s.repo.EXPECT().ListByUser(ctx, "u1", "").Return([]model.TagWithCount{
		{Tag: model.Tag{ID: "t1", Name: "food"}, TxCount: 3},
	}, nil)

	tags, err := s.svc.List(ctx, "u1", "")
	s.NoError(err)
	s.Len(tags, 1)
	s.Equal("food", tags[0].Name)
}

func (s *TagServiceSuite) TestList_WithQuery_PassedThrough() {
	ctx := context.Background()
	s.repo.EXPECT().ListByUser(ctx, "u1", "foo").Return([]model.TagWithCount{}, nil)

	_, err := s.svc.List(ctx, "u1", "foo")
	s.NoError(err)
}

func (s *TagServiceSuite) TestRename_Success() {
	ctx := context.Background()
	original := model.Tag{ID: "t1", UserID: "u1", Name: "old"}

	s.repo.EXPECT().GetByID(ctx, "t1", "u1").Return(original, nil)
	s.repo.EXPECT().Update(ctx, model.Tag{ID: "t1", UserID: "u1", Name: "new"}).Return(
		model.Tag{ID: "t1", UserID: "u1", Name: "new"}, nil,
	)

	t, err := s.svc.Rename(ctx, "u1", "t1", "new")
	s.NoError(err)
	s.Equal("new", t.Name)
}

func (s *TagServiceSuite) TestRename_NotFound() {
	ctx := context.Background()
	s.repo.EXPECT().GetByID(ctx, "t1", "u1").Return(model.Tag{}, apperr.ErrNotFound)

	_, err := s.svc.Rename(ctx, "u1", "t1", "new")
	s.True(errors.Is(err, apperr.ErrNotFound))
}

func (s *TagServiceSuite) TestRename_Conflict() {
	ctx := context.Background()
	s.repo.EXPECT().GetByID(ctx, "t1", "u1").Return(model.Tag{ID: "t1", UserID: "u1", Name: "old"}, nil)
	s.repo.EXPECT().Update(ctx, gomock.Any()).Return(model.Tag{}, apperr.ErrConflict)

	_, err := s.svc.Rename(ctx, "u1", "t1", "duplicate")
	s.True(errors.Is(err, apperr.ErrConflict))
}

func (s *TagServiceSuite) TestDelete_Success() {
	ctx := context.Background()
	s.repo.EXPECT().Delete(ctx, "t1", "u1").Return(nil)

	err := s.svc.Delete(ctx, "u1", "t1")
	s.NoError(err)
}

func (s *TagServiceSuite) TestDelete_NotFound() {
	ctx := context.Background()
	s.repo.EXPECT().Delete(ctx, "t99", "u1").Return(apperr.ErrNotFound)

	err := s.svc.Delete(ctx, "u1", "t99")
	s.True(errors.Is(err, apperr.ErrNotFound))
}

func (s *TagServiceSuite) TestSetHidden_PersonalPreference() {
	for _, hidden := range []bool{true, false} {
		s.repo.EXPECT().GetByID(gomock.Any(), "entry", "viewer").Return(model.Tag{ID: "entry", UserID: "creator"}, nil)
		s.repo.EXPECT().SetHidden(gomock.Any(), "entry", "viewer", hidden).Return(nil)
		s.NoError(s.svc.SetHidden(context.Background(), "viewer", "entry", hidden))
	}
}

func (s *TagServiceSuite) TestSetHidden_NotFound() {
	s.repo.EXPECT().GetByID(gomock.Any(), "missing", "viewer").Return(model.Tag{}, apperr.ErrNotFound)
	s.ErrorIs(s.svc.SetHidden(context.Background(), "viewer", "missing", true), apperr.ErrNotFound)
}

func (s *TagServiceSuite) TestSetHidden_RepositoryError() {
	s.repo.EXPECT().GetByID(gomock.Any(), "entry", "viewer").Return(model.Tag{ID: "entry"}, nil)
	repoErr := errors.New("database unavailable")
	s.repo.EXPECT().SetHidden(gomock.Any(), "entry", "viewer", true).Return(repoErr)
	s.ErrorIs(s.svc.SetHidden(context.Background(), "viewer", "entry", true), repoErr)
}

func (s *TagServiceSuite) TestCreate_NormalizesSharedName() {
	s.repo.EXPECT().Create(gomock.Any(), model.Tag{UserID: "u2", Name: "отпуск"}).Return(model.Tag{ID: "new", Name: "отпуск"}, nil)
	got, err := s.svc.Create(context.Background(), "u2", " Отпуск ")
	s.NoError(err)
	s.Equal("new", got.ID)
}

func (s *TagServiceSuite) TestCreate_InvalidName() {
	_, err := s.svc.Create(context.Background(), "u2", "  ")
	s.ErrorIs(err, apperr.ErrValidation)
}

func (s *TagServiceSuite) TestCreate_Conflict() {
	s.repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(model.Tag{}, apperr.ErrConflict)
	_, err := s.svc.Create(context.Background(), "u2", "duplicate")
	s.ErrorIs(err, apperr.ErrConflict)
}

func (s *TagServiceSuite) TestDelete_LinkedTransactionsConflict() {
	s.repo.EXPECT().Delete(gomock.Any(), "t1", "u2").Return(apperr.ErrConflict)
	s.ErrorIs(s.svc.Delete(context.Background(), "u2", "t1"), apperr.ErrConflict)
}

func (s *TagServiceSuite) TestRename_OtherCreator() {
	s.repo.EXPECT().GetByID(gomock.Any(), "t1", "u2").Return(model.Tag{ID: "t1", UserID: "u1"}, nil)
	s.repo.EXPECT().Update(gomock.Any(), model.Tag{ID: "t1", UserID: "u1", Name: "new"}).Return(model.Tag{ID: "t1", Name: "new"}, nil)
	got, err := s.svc.Rename(context.Background(), "u2", "t1", " New ")
	s.NoError(err)
	s.Equal("new", got.Name)
}
