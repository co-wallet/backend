package currencyhandler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	currencyhandler "github.com/co-wallet/backend/internal/handler/currency"
	"github.com/co-wallet/backend/internal/handler/currency/mocks"
	"github.com/co-wallet/backend/internal/model"
)

type CurrencyHandlerSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	svc  *mocks.MockcurrencyService
	h    *currencyhandler.Handler
}

func TestCurrencyHandlerSuite(t *testing.T) {
	suite.Run(t, new(CurrencyHandlerSuite))
}

func (s *CurrencyHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.svc = mocks.NewMockcurrencyService(s.ctrl)
	s.h = currencyhandler.New(s.svc)
}

func (s *CurrencyHandlerSuite) TestList_NoCodes() {
	s.svc.EXPECT().
		ListActive(gomock.Any(), nil).
		Return([]model.CurrencyWithRate{{Currency: model.Currency{Code: "USD"}, RateToUSD: 1}}, nil)

	rec := httptest.NewRecorder()
	s.h.List(rec, httptest.NewRequest(http.MethodGet, "/currencies", nil))
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"USD"`)
}

func (s *CurrencyHandlerSuite) TestList_ExtraCodes() {
	s.svc.EXPECT().
		ListActive(gomock.Any(), []string{"EUR", "JPY"}).
		Return([]model.CurrencyWithRate{}, nil)

	rec := httptest.NewRecorder()
	s.h.List(rec, httptest.NewRequest(http.MethodGet, "/currencies?codes=EUR,JPY", nil))
	s.Equal(http.StatusOK, rec.Code)
}
