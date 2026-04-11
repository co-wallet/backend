package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service/mocks"
)

type CurrencyServiceSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	repo *mocks.MockcurrencyRepo
	svc  *CurrencyService
}

func TestCurrencyServiceSuite(t *testing.T) {
	suite.Run(t, new(CurrencyServiceSuite))
}

func (s *CurrencyServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockcurrencyRepo(s.ctrl)
	s.svc = NewCurrencyService(s.repo)
}

func (s *CurrencyServiceSuite) TestListActive_ReturnsRepoResult() {
	expected := []model.CurrencyWithRate{
		{Currency: model.Currency{Code: "USD", Name: "Dollar", IsActive: true}, RateToUSD: 1},
		{Currency: model.Currency{Code: "EUR", Name: "Euro", IsActive: true}, RateToUSD: 0.92},
	}
	s.repo.EXPECT().ListActive(gomock.Any(), []string{"RUB"}).Return(expected, nil)

	got, err := s.svc.ListActive(context.Background(), []string{"RUB"})
	s.NoError(err)
	s.Equal(expected, got)
}

func (s *CurrencyServiceSuite) TestListActive_NilNormalizedToEmptySlice() {
	s.repo.EXPECT().ListActive(gomock.Any(), gomock.Nil()).Return(nil, nil)

	got, err := s.svc.ListActive(context.Background(), nil)
	s.NoError(err)
	s.NotNil(got)
	s.Empty(got)
}

func (s *CurrencyServiceSuite) TestListActive_RepoErrorPropagates() {
	s.repo.EXPECT().ListActive(gomock.Any(), gomock.Any()).Return(nil, errors.New("db down"))

	_, err := s.svc.ListActive(context.Background(), nil)
	s.Error(err)
}

func (s *CurrencyServiceSuite) TestGetRate_DelegatesToRepo() {
	s.repo.EXPECT().GetRate(gomock.Any(), "USD", "EUR").Return(0.92, nil)

	rate, err := s.svc.GetRate(context.Background(), "USD", "EUR")
	s.NoError(err)
	s.Equal(0.92, rate)
}
