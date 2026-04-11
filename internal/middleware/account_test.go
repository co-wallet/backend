package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/middleware/mocks"
)

type AccountMemberMiddlewareSuite struct {
	suite.Suite
	ctrl    *gomock.Controller
	checker *mocks.MockmemberChecker
}

func TestAccountMemberMiddlewareSuite(t *testing.T) {
	suite.Run(t, new(AccountMemberMiddlewareSuite))
}

func (s *AccountMemberMiddlewareSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.checker = mocks.NewMockmemberChecker(s.ctrl)
}

func (s *AccountMemberMiddlewareSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *AccountMemberMiddlewareSuite) serve(accountID, userID string) *httptest.ResponseRecorder {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	router := chi.NewRouter()
	router.With(middleware.AccountMember(s.checker)).Get("/accounts/{accountID}", next)

	req := httptest.NewRequest(http.MethodGet, "/accounts/"+accountID, nil)
	if userID != "" {
		ctx := context.WithValue(req.Context(), middleware.ContextUserID, userID)
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK && !nextCalled {
		s.T().Fatalf("next handler not called despite 200")
	}
	return rec
}

func (s *AccountMemberMiddlewareSuite) TestMemberAllowed() {
	s.checker.EXPECT().
		IsMember(gomock.Any(), "acc-1", "user-1").
		Return(true, nil)

	rec := s.serve("acc-1", "user-1")

	s.Equal(http.StatusOK, rec.Code)
}

func (s *AccountMemberMiddlewareSuite) TestNonMemberForbidden() {
	s.checker.EXPECT().
		IsMember(gomock.Any(), "acc-1", "user-1").
		Return(false, nil)

	rec := s.serve("acc-1", "user-1")

	s.Equal(http.StatusForbidden, rec.Code)
}

func (s *AccountMemberMiddlewareSuite) TestRepoErrorForbidden() {
	s.checker.EXPECT().
		IsMember(gomock.Any(), "acc-1", "user-1").
		Return(false, errors.New("db down"))

	rec := s.serve("acc-1", "user-1")

	s.Equal(http.StatusForbidden, rec.Code)
}
