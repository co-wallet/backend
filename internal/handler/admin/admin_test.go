package admin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/handler/admin"
	"github.com/co-wallet/backend/internal/handler/admin/mocks"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type AdminHandlerSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	svc  *mocks.MockadminService
	h    *admin.Handler
}

func TestAdminHandlerSuite(t *testing.T) {
	suite.Run(t, new(AdminHandlerSuite))
}

func (s *AdminHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.svc = mocks.NewMockadminService(s.ctrl)
	s.h = admin.New(s.svc)
}

func withParam(req *http.Request, key, val string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, val)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
}

func (s *AdminHandlerSuite) TestListUsers_Success() {
	s.svc.EXPECT().
		ListUsers(gomock.Any()).
		Return([]model.User{{ID: "u1", Username: "alice"}}, nil)

	rec := httptest.NewRecorder()
	s.h.ListUsers(rec, httptest.NewRequest(http.MethodGet, "/admin/users", nil))
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"alice"`)
}

func (s *AdminHandlerSuite) TestGetUser_NotFound() {
	s.svc.EXPECT().
		GetUser(gomock.Any(), "u1").
		Return(model.User{}, apperr.ErrNotFound)

	req := withParam(httptest.NewRequest(http.MethodGet, "/admin/users/u1", nil), "userID", "u1")
	rec := httptest.NewRecorder()
	s.h.GetUser(rec, req)
	s.Equal(http.StatusNotFound, rec.Code)
}

func (s *AdminHandlerSuite) TestUpdateUser_Success() {
	s.svc.EXPECT().
		UpdateUser(gomock.Any(), "u1", gomock.AssignableToTypeOf(service.AdminUpdateUserReq{})).
		DoAndReturn(func(_ context.Context, _ string, req service.AdminUpdateUserReq) error {
			s.NotNil(req.IsActive)
			s.False(*req.IsActive)
			return nil
		})

	body := `{"isActive":false}`
	req := withParam(httptest.NewRequest(http.MethodPatch, "/admin/users/u1", strings.NewReader(body)), "userID", "u1")
	rec := httptest.NewRecorder()
	s.h.UpdateUser(rec, req)
	s.Equal(http.StatusNoContent, rec.Code)
}

func (s *AdminHandlerSuite) TestUpdateUser_InvalidJSON() {
	req := withParam(httptest.NewRequest(http.MethodPatch, "/admin/users/u1", strings.NewReader(`{`)), "userID", "u1")
	rec := httptest.NewRecorder()
	s.h.UpdateUser(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AdminHandlerSuite) TestListCurrencies_Success() {
	s.svc.EXPECT().
		ListAllCurrencies(gomock.Any()).
		Return([]model.CurrencyWithRate{{Currency: model.Currency{Code: "USD"}, RateToUSD: 1}}, nil)

	rec := httptest.NewRecorder()
	s.h.ListCurrencies(rec, httptest.NewRequest(http.MethodGet, "/admin/currencies", nil))
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"USD"`)
}

func (s *AdminHandlerSuite) TestCreateCurrency_Success() {
	s.svc.EXPECT().
		CreateCurrency(gomock.Any(), gomock.AssignableToTypeOf(service.CreateCurrencyReq{})).
		Return(nil)

	body := `{"code":"EUR","name":"Euro","isActive":true}`
	req := httptest.NewRequest(http.MethodPost, "/admin/currencies", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.h.CreateCurrency(rec, req)
	s.Equal(http.StatusCreated, rec.Code)
}

func (s *AdminHandlerSuite) TestCreateCurrency_Validation() {
	s.svc.EXPECT().
		CreateCurrency(gomock.Any(), gomock.Any()).
		Return(apperr.ErrValidation)

	body := `{"code":"XYZ","name":"x","isActive":true}`
	req := httptest.NewRequest(http.MethodPost, "/admin/currencies", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.h.CreateCurrency(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AdminHandlerSuite) TestUpdateCurrency_Success() {
	s.svc.EXPECT().
		UpdateCurrency(gomock.Any(), "USD", gomock.AssignableToTypeOf(service.UpdateCurrencyReq{})).
		Return(nil)

	body := `{"isActive":false}`
	req := withParam(httptest.NewRequest(http.MethodPatch, "/admin/currencies/USD", strings.NewReader(body)), "code", "USD")
	rec := httptest.NewRecorder()
	s.h.UpdateCurrency(rec, req)
	s.Equal(http.StatusNoContent, rec.Code)
}

func (s *AdminHandlerSuite) TestRefreshRates_Success() {
	s.svc.EXPECT().
		RefreshRates(gomock.Any()).
		Return(nil)

	rec := httptest.NewRecorder()
	s.h.RefreshRates(rec, httptest.NewRequest(http.MethodPost, "/admin/currencies/rates/refresh", nil))
	s.Equal(http.StatusNoContent, rec.Code)
}
