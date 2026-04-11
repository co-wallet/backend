package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/middleware/mocks"
	"github.com/co-wallet/backend/internal/service"
)

type AuthMiddlewareSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	validator *mocks.MocktokenValidator
}

func TestAuthMiddlewareSuite(t *testing.T) {
	suite.Run(t, new(AuthMiddlewareSuite))
}

func (s *AuthMiddlewareSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.validator = mocks.NewMocktokenValidator(s.ctrl)
}

func (s *AuthMiddlewareSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *AuthMiddlewareSuite) serve(req *http.Request) (*httptest.ResponseRecorder, string, bool) {
	var gotUserID string
	var gotIsAdmin bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = middleware.UserIDFromCtx(r.Context())
		gotIsAdmin, _ = r.Context().Value(middleware.ContextIsAdmin).(bool)
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	middleware.Auth(s.validator)(next).ServeHTTP(rec, req)
	return rec, gotUserID, gotIsAdmin
}

func (s *AuthMiddlewareSuite) TestValidBearerTokenPopulatesContext() {
	s.validator.EXPECT().
		ValidateAccessToken("good-token").
		Return(&service.Claims{UserID: "user-1", IsAdmin: true}, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer good-token")

	rec, userID, isAdmin := s.serve(req)

	s.Equal(http.StatusOK, rec.Code)
	s.Equal("user-1", userID)
	s.True(isAdmin)
}

func (s *AuthMiddlewareSuite) TestMissingHeaderUnauthorized() {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rec, userID, _ := s.serve(req)

	s.Equal(http.StatusUnauthorized, rec.Code)
	s.Empty(userID)
}

func (s *AuthMiddlewareSuite) TestNonBearerHeaderUnauthorized() {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic abc")

	rec, _, _ := s.serve(req)

	s.Equal(http.StatusUnauthorized, rec.Code)
}

func (s *AuthMiddlewareSuite) TestInvalidTokenUnauthorized() {
	s.validator.EXPECT().
		ValidateAccessToken("bad-token").
		Return(nil, errors.New("invalid"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad-token")

	rec, userID, _ := s.serve(req)

	s.Equal(http.StatusUnauthorized, rec.Code)
	s.Empty(userID)
}

func TestAdminMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		ctxIsAdmin any
		wantStatus int
	}{
		{"admin passes", true, http.StatusOK},
		{"non-admin forbidden", false, http.StatusForbidden},
		{"missing value forbidden", nil, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.ctxIsAdmin != nil {
				ctx := context.WithValue(req.Context(), middleware.ContextIsAdmin, tt.ctxIsAdmin.(bool))
				req = req.WithContext(ctx)
			}
			rec := httptest.NewRecorder()
			middleware.Admin(next).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
