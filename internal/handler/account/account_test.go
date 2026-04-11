package accounthandler_test

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
	accounthandler "github.com/co-wallet/backend/internal/handler/account"
	"github.com/co-wallet/backend/internal/handler/account/mocks"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
)

type AccountHandlerSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	svc   *mocks.MockaccountService
	users *mocks.MockuserSource
	h     *accounthandler.Handler
}

func TestAccountHandlerSuite(t *testing.T) {
	suite.Run(t, new(AccountHandlerSuite))
}

func (s *AccountHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.svc = mocks.NewMockaccountService(s.ctrl)
	s.users = mocks.NewMockuserSource(s.ctrl)
	s.h = accounthandler.New(s.svc, s.users)
}

func withUser(req *http.Request, id string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.ContextUserID, id))
}

func withAccountParam(req *http.Request, id string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("accountID", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
}

func withAccountAndUserParams(req *http.Request, accountID, userID string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("accountID", accountID)
	rc.URLParams.Add("userID", userID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
}

func (s *AccountHandlerSuite) TestList_WithDefaultCurrency() {
	s.users.EXPECT().
		GetByID(gomock.Any(), "u1").
		Return(model.User{ID: "u1", DefaultCurrency: "USD"}, nil)
	s.svc.EXPECT().
		ListByUser(gomock.Any(), "u1").
		Return([]model.Account{{ID: "a1", Type: model.AccountTypePersonal}}, nil)
	s.svc.EXPECT().
		ListBalancesByUser(gomock.Any(), "u1", "USD").
		Return(map[string]model.AccountBalance{
			"a1": {AccountID: "a1", BalanceNative: 10, BalanceDisplay: 10, TotalNative: 10, TotalDisplay: 10},
		}, nil)

	req := withUser(httptest.NewRequest(http.MethodGet, "/accounts", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.List(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"a1"`)
	s.Contains(rec.Body.String(), `"displayCurrency":"USD"`)
}

func (s *AccountHandlerSuite) TestList_CurrencyFromQuery() {
	s.svc.EXPECT().
		ListByUser(gomock.Any(), "u1").
		Return([]model.Account{}, nil)
	s.svc.EXPECT().
		ListBalancesByUser(gomock.Any(), "u1", "EUR").
		Return(map[string]model.AccountBalance{}, nil)

	req := withUser(httptest.NewRequest(http.MethodGet, "/accounts?currency=EUR", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.List(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *AccountHandlerSuite) TestCreate_Success() {
	s.svc.EXPECT().
		CreateAccount(gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, req model.CreateAccountReq) (model.Account, error) {
			s.Equal("Wallet", req.Name)
			s.Equal("USD", req.Currency)
			s.Equal(model.AccountTypePersonal, req.Type)
			return model.Account{ID: "a1", Name: "Wallet", Type: model.AccountTypePersonal, Currency: "USD"}, nil
		})

	body := `{"name":"Wallet","type":"personal","currency":"usd","initialBalanceDate":"2025-01-01"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusCreated, rec.Code)
	s.Contains(rec.Body.String(), `"a1"`)
}

func (s *AccountHandlerSuite) TestCreate_ValidationError() {
	body := `{"name":"","type":"personal","currency":"USD","initialBalanceDate":"2025-01-01"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AccountHandlerSuite) TestCreate_InvalidJSON() {
	req := withUser(httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{`)), "u1")
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AccountHandlerSuite) TestGet_PersonalSuccess() {
	s.svc.EXPECT().
		GetByID(gomock.Any(), "a1").
		Return(model.Account{ID: "a1", Type: model.AccountTypePersonal}, nil)

	req := withAccountParam(withUser(httptest.NewRequest(http.MethodGet, "/accounts/a1", nil), "u1"), "a1")
	rec := httptest.NewRecorder()
	s.h.Get(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"a1"`)
}

func (s *AccountHandlerSuite) TestGet_SharedWithMembers() {
	s.svc.EXPECT().
		GetByID(gomock.Any(), "a1").
		Return(model.Account{ID: "a1", Type: model.AccountTypeShared}, nil)
	s.svc.EXPECT().
		GetMembers(gomock.Any(), "a1").
		Return([]model.AccountMember{{AccountID: "a1", UserID: "u1", Username: "alice", DefaultShare: 1}}, nil)

	req := withAccountParam(withUser(httptest.NewRequest(http.MethodGet, "/accounts/a1", nil), "u1"), "a1")
	rec := httptest.NewRecorder()
	s.h.Get(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"alice"`)
}

func (s *AccountHandlerSuite) TestGet_NotFound() {
	s.svc.EXPECT().
		GetByID(gomock.Any(), "a1").
		Return(model.Account{}, apperr.ErrNotFound)

	req := withAccountParam(withUser(httptest.NewRequest(http.MethodGet, "/accounts/a1", nil), "u1"), "a1")
	rec := httptest.NewRecorder()
	s.h.Get(rec, req)
	s.Equal(http.StatusNotFound, rec.Code)
}

func (s *AccountHandlerSuite) TestUpdate_Success() {
	s.svc.EXPECT().
		UpdateAccount(gomock.Any(), "a1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, req model.UpdateAccountReq) (model.Account, error) {
			s.NotNil(req.Name)
			s.Equal("New", *req.Name)
			return model.Account{ID: "a1", Name: "New", Type: model.AccountTypePersonal}, nil
		})

	body := `{"name":"New"}`
	req := withAccountParam(withUser(httptest.NewRequest(http.MethodPatch, "/accounts/a1", strings.NewReader(body)), "u1"), "a1")
	rec := httptest.NewRecorder()
	s.h.Update(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *AccountHandlerSuite) TestUpdate_InvalidDate() {
	body := `{"initialBalanceDate":"not-a-date"}`
	req := withAccountParam(withUser(httptest.NewRequest(http.MethodPatch, "/accounts/a1", strings.NewReader(body)), "u1"), "a1")
	rec := httptest.NewRecorder()
	s.h.Update(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AccountHandlerSuite) TestDelete_Success() {
	s.svc.EXPECT().
		DeleteAccount(gomock.Any(), "u1", "a1").
		Return(nil)

	req := withAccountParam(withUser(httptest.NewRequest(http.MethodDelete, "/accounts/a1", nil), "u1"), "a1")
	rec := httptest.NewRecorder()
	s.h.Delete(rec, req)
	s.Equal(http.StatusNoContent, rec.Code)
}

func (s *AccountHandlerSuite) TestDelete_Forbidden() {
	s.svc.EXPECT().
		DeleteAccount(gomock.Any(), "u1", "a1").
		Return(apperr.ErrForbidden)

	req := withAccountParam(withUser(httptest.NewRequest(http.MethodDelete, "/accounts/a1", nil), "u1"), "a1")
	rec := httptest.NewRecorder()
	s.h.Delete(rec, req)
	s.Equal(http.StatusForbidden, rec.Code)
}

func (s *AccountHandlerSuite) TestListMembers_Success() {
	s.svc.EXPECT().
		GetMembers(gomock.Any(), "a1").
		Return([]model.AccountMember{{AccountID: "a1", UserID: "u1"}}, nil)

	req := withAccountParam(httptest.NewRequest(http.MethodGet, "/accounts/a1/members", nil), "a1")
	rec := httptest.NewRecorder()
	s.h.ListMembers(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *AccountHandlerSuite) TestAddMember_Success() {
	s.svc.EXPECT().
		AddMember(gomock.Any(), "a1", "alice", 0.5).
		Return([]model.AccountMember{{UserID: "u1"}, {UserID: "u2"}}, nil)

	body := `{"username":"alice","defaultShare":0.5}`
	req := withAccountParam(httptest.NewRequest(http.MethodPost, "/accounts/a1/members", strings.NewReader(body)), "a1")
	rec := httptest.NewRecorder()
	s.h.AddMember(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *AccountHandlerSuite) TestAddMember_ShareOutOfRange() {
	body := `{"username":"alice","defaultShare":1.5}`
	req := withAccountParam(httptest.NewRequest(http.MethodPost, "/accounts/a1/members", strings.NewReader(body)), "a1")
	rec := httptest.NewRecorder()
	s.h.AddMember(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AccountHandlerSuite) TestUpdateMember_Success() {
	s.svc.EXPECT().
		UpdateMember(gomock.Any(), "a1", "u2", 0.3).
		Return([]model.AccountMember{{UserID: "u2", DefaultShare: 0.3}}, nil)

	body := `{"defaultShare":0.3}`
	req := withAccountAndUserParams(httptest.NewRequest(http.MethodPatch, "/accounts/a1/members/u2", strings.NewReader(body)), "a1", "u2")
	rec := httptest.NewRecorder()
	s.h.UpdateMember(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *AccountHandlerSuite) TestRemoveMember_Success() {
	s.svc.EXPECT().
		RemoveMember(gomock.Any(), "u1", "a1", "u2").
		Return(nil)

	req := withAccountAndUserParams(withUser(httptest.NewRequest(http.MethodDelete, "/accounts/a1/members/u2", nil), "u1"), "a1", "u2")
	rec := httptest.NewRecorder()
	s.h.RemoveMember(rec, req)
	s.Equal(http.StatusNoContent, rec.Code)
}
