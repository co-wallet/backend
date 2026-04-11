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

type UserServiceSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	repo *mocks.MockUserRepo
	svc  *UserService
}

func (s *UserServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockUserRepo(s.ctrl)
	s.svc = NewUserService(s.repo)
}

func TestUserServiceSuite(t *testing.T) {
	suite.Run(t, new(UserServiceSuite))
}

func (s *UserServiceSuite) TestGetByID_Success() {
	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{ID: "u1", Username: "alice"}, nil)

	u, err := s.svc.GetByID(context.Background(), "u1")
	s.NoError(err)
	s.Equal("alice", u.Username)
}

func (s *UserServiceSuite) TestGetByID_NotFound() {
	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{}, apperr.ErrNotFound)

	_, err := s.svc.GetByID(context.Background(), "u1")
	s.True(errors.Is(err, apperr.ErrNotFound))
}

func (s *UserServiceSuite) TestListActive_ReturnsNonNil() {
	s.repo.EXPECT().ListActive(gomock.Any()).Return(nil, nil)

	got, err := s.svc.ListActive(context.Background())
	s.NoError(err)
	s.NotNil(got)
	s.Len(got, 0)
}

func (s *UserServiceSuite) TestListActive_Success() {
	users := []model.User{{ID: "1"}, {ID: "2"}}
	s.repo.EXPECT().ListActive(gomock.Any()).Return(users, nil)

	got, err := s.svc.ListActive(context.Background())
	s.NoError(err)
	s.Len(got, 2)
}

func (s *UserServiceSuite) TestUpdateCurrency_Success() {
	s.repo.EXPECT().UpdateCurrency(gomock.Any(), "u1", "USD").Return(nil)
	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{ID: "u1", DefaultCurrency: "USD"}, nil)

	u, err := s.svc.UpdateCurrency(context.Background(), "u1", "usd")
	s.NoError(err)
	s.Equal("USD", u.DefaultCurrency)
}

func (s *UserServiceSuite) TestUpdateCurrency_NormalizesWhitespaceAndCase() {
	s.repo.EXPECT().UpdateCurrency(gomock.Any(), "u1", "EUR").Return(nil)
	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{ID: "u1"}, nil)

	_, err := s.svc.UpdateCurrency(context.Background(), "u1", "  eur  ")
	s.NoError(err)
}

func (s *UserServiceSuite) TestUpdateCurrency_InvalidLength() {
	_, err := s.svc.UpdateCurrency(context.Background(), "u1", "EURO")
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *UserServiceSuite) TestUpdateCurrency_Empty() {
	_, err := s.svc.UpdateCurrency(context.Background(), "u1", "")
	s.True(errors.Is(err, apperr.ErrValidation))
}
