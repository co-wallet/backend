package analytics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/handler/analytics"
	"github.com/co-wallet/backend/internal/handler/analytics/mocks"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
)

type AnalyticsHandlerSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	svc   *mocks.MockanalyticsService
	users *mocks.MockuserSource
	h     *analytics.Handler
}

func TestAnalyticsHandlerSuite(t *testing.T) {
	suite.Run(t, new(AnalyticsHandlerSuite))
}

func (s *AnalyticsHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.svc = mocks.NewMockanalyticsService(s.ctrl)
	s.users = mocks.NewMockuserSource(s.ctrl)
	s.h = analytics.New(s.svc, s.users)
}

func withUser(req *http.Request, id string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.ContextUserID, id))
}

func (s *AnalyticsHandlerSuite) TestSummary_UsesDefaultCurrency() {
	s.users.EXPECT().
		GetByID(gomock.Any(), "u1").
		Return(model.User{ID: "u1", DefaultCurrency: "EUR"}, nil)
	s.svc.EXPECT().
		Summary(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, f model.AnalyticsFilter) (model.AnalyticsSummary, error) {
			s.Equal("u1", f.UserID)
			s.Equal("EUR", f.DisplayCurrency)
			return model.AnalyticsSummary{Balance: 100, Expenses: 50, Income: 150}, nil
		})

	req := withUser(httptest.NewRequest(http.MethodGet, "/analytics/summary?date_from=2025-01-01&date_to=2025-12-31", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.Summary(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"balance":100`)
}

func (s *AnalyticsHandlerSuite) TestSummary_ExplicitCurrency() {
	s.svc.EXPECT().
		Summary(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, f model.AnalyticsFilter) (model.AnalyticsSummary, error) {
			s.Equal("USD", f.DisplayCurrency)
			return model.AnalyticsSummary{}, nil
		})

	req := withUser(httptest.NewRequest(http.MethodGet, "/analytics/summary?currency=USD", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.Summary(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *AnalyticsHandlerSuite) TestSummary_InvalidDate() {
	req := withUser(httptest.NewRequest(http.MethodGet, "/analytics/summary?date_from=not-a-date", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.Summary(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AnalyticsHandlerSuite) TestSummary_InvalidCurrency() {
	req := withUser(httptest.NewRequest(http.MethodGet, "/analytics/summary?currency=US", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.Summary(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AnalyticsHandlerSuite) TestByCategory_Success() {
	s.users.EXPECT().
		GetByID(gomock.Any(), "u1").
		Return(model.User{DefaultCurrency: "USD"}, nil)
	s.svc.EXPECT().
		ByCategory(gomock.Any(), gomock.Any()).
		Return([]model.CategoryStat{{CategoryID: "c1", CategoryName: "Food", Amount: 42}}, nil)

	req := withUser(httptest.NewRequest(http.MethodGet, "/analytics/by-category", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.ByCategory(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"Food"`)
}

func (s *AnalyticsHandlerSuite) TestByTag_Success() {
	s.users.EXPECT().
		GetByID(gomock.Any(), "u1").
		Return(model.User{DefaultCurrency: "USD"}, nil)
	s.svc.EXPECT().
		ByTag(gomock.Any(), gomock.Any()).
		Return([]model.TagStat{{TagID: "t1", TagName: "lunch", Amount: 12}}, nil)

	req := withUser(httptest.NewRequest(http.MethodGet, "/analytics/by-tag", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.ByTag(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"lunch"`)
}
