package transactionhandler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/apperr"
	transactionhandler "github.com/co-wallet/backend/internal/handler/transaction"
	"github.com/co-wallet/backend/internal/handler/transaction/mocks"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
)

type TransactionHandlerSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	svc  *mocks.MocktransactionService
	h    *transactionhandler.Handler
}

func TestTransactionHandlerSuite(t *testing.T) {
	suite.Run(t, new(TransactionHandlerSuite))
}

func (s *TransactionHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.svc = mocks.NewMocktransactionService(s.ctrl)
	s.h = transactionhandler.New(s.svc)
}

func withUser(req *http.Request, userID string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.ContextUserID, userID))
}

func withTxIDParam(req *http.Request, id string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("transactionID", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
}

func (s *TransactionHandlerSuite) TestList_AppliesFilters() {
	s.svc.EXPECT().
		List(gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, f model.TransactionFilter) ([]model.Transaction, error) {
			s.Equal([]string{"a1", "a2"}, f.AccountIDs)
			s.Equal([]string{"c1"}, f.CategoryIDs)
			s.Equal([]string{"t1"}, f.TagIDs)
			s.Equal("and", f.TagMode)
			s.Equal(2, f.Page)
			s.Equal(10, f.Limit)
			s.NotNil(f.DateFrom)
			s.NotNil(f.DateTo)
			return []model.Transaction{{ID: "tx1"}}, nil
		})

	req := withUser(httptest.NewRequest(http.MethodGet, "/transactions?account_ids=a1,a2&category_ids=c1&tag_ids=t1&tag_mode=and&date_from=2025-01-01&date_to=2025-12-31&page=2&limit=10", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.List(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"tx1"`)
}

func (s *TransactionHandlerSuite) TestList_DefaultsPageLimit() {
	s.svc.EXPECT().
		List(gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, f model.TransactionFilter) ([]model.Transaction, error) {
			s.Equal(1, f.Page)
			s.Equal(50, f.Limit)
			s.Empty(f.TagMode)
			return nil, nil
		})

	req := withUser(httptest.NewRequest(http.MethodGet, "/transactions", nil), "u1")
	s.h.List(httptest.NewRecorder(), req)
}

func (s *TransactionHandlerSuite) TestCreate_Success() {
	s.svc.EXPECT().
		Create(gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, req model.CreateTransactionReq) (model.Transaction, error) {
			s.Equal("acc-1", req.AccountID)
			s.Equal(model.TransactionTypeExpense, req.Type)
			s.Equal(100.0, req.Amount)
			s.Equal("USD", req.Currency)
			return model.Transaction{ID: "tx1", Type: model.TransactionTypeExpense, Amount: 100, Currency: "USD", Date: req.Date}, nil
		})

	body := `{"accountId":"acc-1","type":"expense","amount":100,"currency":"usd","date":"2025-05-01T00:00:00Z"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusCreated, rec.Code)
	s.Contains(rec.Body.String(), `"tx1"`)
}

func (s *TransactionHandlerSuite) TestCreate_ValidationError() {
	body := `{"accountId":"","type":"expense","amount":100,"currency":"USD","date":"2025-05-01T00:00:00Z"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
	s.Contains(rec.Body.String(), "accountId is required")
}

func (s *TransactionHandlerSuite) TestCreate_InvalidJSON() {
	req := withUser(httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(`{`)), "u1")
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *TransactionHandlerSuite) TestCreate_TransferMissingTo() {
	body := `{"accountId":"acc-1","type":"transfer","amount":100,"currency":"USD","date":"2025-05-01T00:00:00Z"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
	s.Contains(rec.Body.String(), "toAccountId is required")
}

func (s *TransactionHandlerSuite) TestGet_Success() {
	s.svc.EXPECT().
		GetByID(gomock.Any(), "u1", "tx1").
		Return(model.Transaction{ID: "tx1", Date: time.Now()}, nil)

	req := withTxIDParam(withUser(httptest.NewRequest(http.MethodGet, "/transactions/tx1", nil), "u1"), "tx1")
	rec := httptest.NewRecorder()
	s.h.Get(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"tx1"`)
}

func (s *TransactionHandlerSuite) TestGet_Forbidden() {
	s.svc.EXPECT().
		GetByID(gomock.Any(), "u1", "tx1").
		Return(model.Transaction{}, apperr.ErrForbidden)

	req := withTxIDParam(withUser(httptest.NewRequest(http.MethodGet, "/transactions/tx1", nil), "u1"), "tx1")
	rec := httptest.NewRecorder()
	s.h.Get(rec, req)
	s.Equal(http.StatusForbidden, rec.Code)
}

func (s *TransactionHandlerSuite) TestUpdate_Success() {
	s.svc.EXPECT().
		Update(gomock.Any(), "u1", "tx1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, req model.UpdateTransactionReq) (model.Transaction, error) {
			s.NotNil(req.Amount)
			s.Equal(55.0, *req.Amount)
			return model.Transaction{ID: "tx1", Amount: 55}, nil
		})

	body := `{"amount":55}`
	req := withTxIDParam(withUser(httptest.NewRequest(http.MethodPatch, "/transactions/tx1", strings.NewReader(body)), "u1"), "tx1")
	rec := httptest.NewRecorder()
	s.h.Update(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *TransactionHandlerSuite) TestUpdate_NegativeAmount() {
	body := `{"amount":-1}`
	req := withTxIDParam(withUser(httptest.NewRequest(http.MethodPatch, "/transactions/tx1", strings.NewReader(body)), "u1"), "tx1")
	rec := httptest.NewRecorder()
	s.h.Update(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *TransactionHandlerSuite) TestDelete_Success() {
	s.svc.EXPECT().
		Delete(gomock.Any(), "u1", "tx1").
		Return(nil)

	req := withTxIDParam(withUser(httptest.NewRequest(http.MethodDelete, "/transactions/tx1", nil), "u1"), "tx1")
	rec := httptest.NewRecorder()
	s.h.Delete(rec, req)
	s.Equal(http.StatusNoContent, rec.Code)
}

func (s *TransactionHandlerSuite) TestDelete_NotFound() {
	s.svc.EXPECT().
		Delete(gomock.Any(), "u1", "tx1").
		Return(apperr.ErrNotFound)

	req := withTxIDParam(withUser(httptest.NewRequest(http.MethodDelete, "/transactions/tx1", nil), "u1"), "tx1")
	rec := httptest.NewRecorder()
	s.h.Delete(rec, req)
	s.Equal(http.StatusNotFound, rec.Code)
}
