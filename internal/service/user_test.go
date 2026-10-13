package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
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

func (s *UserServiceSuite) TestUpdateProfile_Validation() {
	cases := []UpdateProfileReq{
		{}, {Username: ptr.To("  ")}, {Username: ptr.To(strings.Repeat("я", 51))},
		{Email: ptr.To("bad")}, {Email: ptr.To("Alice <a@example.com>")},
		{DefaultCurrency: ptr.To("123")}, {NewPassword: ptr.To("short")},
		{NewPassword: ptr.To(strings.Repeat("x", 73))},
	}
	for i, req := range cases {
		s.Run(fmt.Sprint(i), func() {
			_, err := s.svc.UpdateProfile(context.Background(), "u1", req)
			s.ErrorIs(err, apperr.ErrValidation)
		})
	}
}
func (s *UserServiceSuite) TestUpdateProfile_NameOnly() {
	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{ID: "u1", Email: "a@test.com"}, nil)
	s.repo.EXPECT().UpdateProfile(gomock.Any(), "u1", model.UserProfilePatch{Username: ptr.To("Алиса")}, "").Return(model.User{ID: "u1", Username: "Алиса"}, nil)
	got, err := s.svc.UpdateProfile(context.Background(), "u1", UpdateProfileReq{Username: ptr.To(" Алиса ")})
	s.NoError(err)
	s.Equal("Алиса", got.Username)
}
func (s *UserServiceSuite) TestUpdateProfile_Currency() {
	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{ID: "u1"}, nil)
	s.repo.EXPECT().UpdateProfile(gomock.Any(), "u1", model.UserProfilePatch{DefaultCurrency: ptr.To("EUR")}, "").Return(model.User{DefaultCurrency: "EUR"}, nil)
	got, err := s.svc.UpdateProfile(context.Background(), "u1", UpdateProfileReq{DefaultCurrency: ptr.To(" eur ")})
	s.NoError(err)
	s.Equal("EUR", got.DefaultCurrency)
}
func (s *UserServiceSuite) TestUpdateProfile_EmailAndPassword() {
	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	s.NoError(err)
	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{ID: "u1", Email: "old@test.com", PasswordHash: string(hash)}, nil)
	s.repo.EXPECT().UpdateProfile(gomock.Any(), "u1", gomock.Any(), string(hash)).DoAndReturn(func(_ context.Context, _ string, patch model.UserProfilePatch, _ string) (model.User, error) {
		s.Equal("new@test.com", *patch.Email)
		s.NoError(bcrypt.CompareHashAndPassword([]byte(*patch.PasswordHash), []byte("new-password")))
		s.NotEqual("new-password", *patch.PasswordHash)
		return model.User{ID: "u1", Email: *patch.Email}, nil
	})
	got, err := s.svc.UpdateProfile(context.Background(), "u1", UpdateProfileReq{Email: ptr.To(" NEW@Test.com "), NewPassword: ptr.To("new-password"), CurrentPassword: "old-password"})
	s.NoError(err)
	s.Equal("new@test.com", got.Email)
}
func (s *UserServiceSuite) TestUpdateProfile_WrongOrMissingCurrentPassword() {
	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	s.NoError(err)
	for _, req := range []UpdateProfileReq{{Email: ptr.To("new@test.com")}, {NewPassword: ptr.To("new-password"), CurrentPassword: "wrong"}} {
		s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{Email: "old@test.com", PasswordHash: string(hash)}, nil)
		_, err := s.svc.UpdateProfile(context.Background(), "u1", req)
		s.ErrorIs(err, apperr.ErrValidation)
	}
}
func (s *UserServiceSuite) TestUpdateProfile_UnchangedEmailNeedsNoPassword() {
	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{Email: "a@test.com"}, nil)
	s.repo.EXPECT().UpdateProfile(gomock.Any(), "u1", model.UserProfilePatch{}, "").Return(model.User{Email: "a@test.com"}, nil)
	_, err := s.svc.UpdateProfile(context.Background(), "u1", UpdateProfileReq{Email: ptr.To(" A@Test.com ")})
	s.NoError(err)
}
func (s *UserServiceSuite) TestUpdateProfile_Errors() {
	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{}, apperr.ErrNotFound)
	_, err := s.svc.UpdateProfile(context.Background(), "u1", UpdateProfileReq{Username: ptr.To("alice")})
	s.ErrorIs(err, apperr.ErrNotFound)
	for _, failure := range []error{apperr.ErrConflict, errors.New("database offline")} {
		s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{}, nil)
		s.repo.EXPECT().UpdateProfile(gomock.Any(), "u1", gomock.Any(), "").Return(model.User{}, failure)
		_, err := s.svc.UpdateProfile(context.Background(), "u1", UpdateProfileReq{Username: ptr.To("alice")})
		s.ErrorIs(err, failure)
	}
}
