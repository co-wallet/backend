package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
	"github.com/co-wallet/backend/internal/service/mocks"
)

type AdminServiceSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	repo  *mocks.MockadminRepo
	rates *mocks.MockratesRefresher
	svc   *AdminService
}

func TestAdminServiceSuite(t *testing.T) {
	suite.Run(t, new(AdminServiceSuite))
}

func (s *AdminServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockadminRepo(s.ctrl)
	s.rates = mocks.NewMockratesRefresher(s.ctrl)
	s.svc = NewAdminService(s.repo, s.rates)
}

func (s *AdminServiceSuite) TestUpdateUser_FlagsOnly() {
	req := AdminUpdateUserReq{IsActive: ptr.To(false), IsAdmin: ptr.To(true)}
	s.repo.EXPECT().
		UpdateUser(gomock.Any(), "u1", gomock.AssignableToTypeOf(model.AdminUserPatch{})).
		DoAndReturn(func(_ context.Context, _ string, patch model.AdminUserPatch) error {
			s.NotNil(patch.IsActive)
			s.False(*patch.IsActive)
			s.NotNil(patch.IsAdmin)
			s.True(*patch.IsAdmin)
			s.Nil(patch.PasswordHash)
			return nil
		})

	err := s.svc.UpdateUser(context.Background(), "u1", req)
	s.NoError(err)
}

func (s *AdminServiceSuite) TestUpdateUser_HashesNewPassword() {
	req := AdminUpdateUserReq{NewPassword: ptr.To("new-strong-pw")}
	s.repo.EXPECT().
		UpdateUser(gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, patch model.AdminUserPatch) error {
			s.Require().NotNil(patch.PasswordHash)
			// bcrypt hash must verify against the plaintext
			err := bcrypt.CompareHashAndPassword([]byte(*patch.PasswordHash), []byte("new-strong-pw"))
			s.NoError(err)
			return nil
		})

	err := s.svc.UpdateUser(context.Background(), "u1", req)
	s.NoError(err)
}

func (s *AdminServiceSuite) TestUpdateUser_EmptyPasswordIgnored() {
	req := AdminUpdateUserReq{NewPassword: ptr.To("")}
	s.repo.EXPECT().
		UpdateUser(gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, patch model.AdminUserPatch) error {
			s.Nil(patch.PasswordHash)
			return nil
		})

	err := s.svc.UpdateUser(context.Background(), "u1", req)
	s.NoError(err)
}

func (s *AdminServiceSuite) TestCreateCurrency_UnknownCodeRejected() {
	s.repo.EXPECT().RateKnown(gomock.Any(), "ZZZ").Return(false, nil)

	err := s.svc.CreateCurrency(context.Background(), CreateCurrencyReq{Code: "ZZZ", Name: "Zog"})
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *AdminServiceSuite) TestCreateCurrency_Success() {
	s.repo.EXPECT().RateKnown(gomock.Any(), "EUR").Return(true, nil)
	s.repo.EXPECT().
		CreateCurrency(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, c model.Currency) error {
			s.Equal("EUR", c.Code)
			s.Equal("Euro", c.Name)
			s.True(c.IsActive)
			return nil
		})

	err := s.svc.CreateCurrency(context.Background(), CreateCurrencyReq{
		Code: "EUR", Name: "Euro", IsActive: true,
	})
	s.NoError(err)
}

func (s *AdminServiceSuite) TestCreateCurrency_RateKnownError() {
	s.repo.EXPECT().RateKnown(gomock.Any(), "EUR").Return(false, errors.New("db down"))

	err := s.svc.CreateCurrency(context.Background(), CreateCurrencyReq{Code: "EUR", Name: "Euro"})
	s.Error(err)
	s.False(errors.Is(err, apperr.ErrValidation))
}

func (s *AdminServiceSuite) TestRefreshRates_DelegatesToRefresher() {
	s.rates.EXPECT().FetchAndStoreRates(gomock.Any()).Return(nil)

	s.NoError(s.svc.RefreshRates(context.Background()))
}

func (s *AdminServiceSuite) TestUpdateCurrency_PropagatesPatch() {
	req := UpdateCurrencyReq{Name: ptr.To("Euro EU"), IsActive: ptr.To(false)}
	s.repo.EXPECT().
		UpdateCurrency(gomock.Any(), "EUR", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, patch model.CurrencyPatch) error {
			s.Require().NotNil(patch.Name)
			s.Equal("Euro EU", *patch.Name)
			s.Require().NotNil(patch.IsActive)
			s.False(*patch.IsActive)
			s.Nil(patch.Symbol)
			return nil
		})

	s.NoError(s.svc.UpdateCurrency(context.Background(), "EUR", req))
}
