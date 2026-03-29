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
