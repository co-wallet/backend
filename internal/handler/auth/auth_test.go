package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/handler/auth"
	"github.com/co-wallet/backend/internal/handler/auth/mocks"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type AuthHandlerSuite struct {
	suite.Suite
	ctrl    *gomock.Controller
	authSvc *mocks.MockauthService
	userSvc *mocks.MockuserService
	h       *auth.Handler
}

func TestAuthHandlerSuite(t *testing.T) {
	suite.Run(t, new(AuthHandlerSuite))
}

func (s *AuthHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.authSvc = mocks.NewMockauthService(s.ctrl)
	s.userSvc = mocks.NewMockuserService(s.ctrl)
	s.h = auth.New(s.authSvc, s.userSvc)
}

func (s *AuthHandlerSuite) TestLogin_Success() {
	user := model.User{ID: "u1", Email: "a@b.c", Username: "alice"}
	tokens := service.TokenPair{AccessToken: "acc", RefreshToken: "ref"}

	s.authSvc.EXPECT().
		Login(gomock.Any(), "a@b.c", "pw").
		Return(user, tokens, nil)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":" A@B.C ","password":"pw"}`))
	rec := httptest.NewRecorder()
	s.h.Login(rec, req)

	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"accessToken":"acc"`)
	s.Contains(rec.Body.String(), `"id":"u1"`)
}

func (s *AuthHandlerSuite) TestLogin_InvalidJSON() {
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`not json`))
	rec := httptest.NewRecorder()
	s.h.Login(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AuthHandlerSuite) TestLogin_WrongCredentials() {
	s.authSvc.EXPECT().
		Login(gomock.Any(), "a@b.c", "pw").
		Return(model.User{}, service.TokenPair{}, apperr.ErrUnauthorized)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"a@b.c","password":"pw"}`))
	rec := httptest.NewRecorder()
	s.h.Login(rec, req)
	s.Equal(http.StatusUnauthorized, rec.Code)
}

func (s *AuthHandlerSuite) TestRefresh_Success() {
	s.authSvc.EXPECT().
		Refresh(gomock.Any(), "tok").
		Return(service.TokenPair{AccessToken: "a", RefreshToken: "r"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{"refreshToken":"tok"}`))
	rec := httptest.NewRecorder()
	s.h.Refresh(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"accessToken":"a"`)
}

func (s *AuthHandlerSuite) TestRefresh_EmptyToken() {
	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{"refreshToken":""}`))
	rec := httptest.NewRecorder()
	s.h.Refresh(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AuthHandlerSuite) TestRefresh_InvalidJSON() {
	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`nope`))
	rec := httptest.NewRecorder()
	s.h.Refresh(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AuthHandlerSuite) TestMe_Success() {
	ctx := context.WithValue(context.Background(), middleware.ContextUserID, "u1")
	s.userSvc.EXPECT().
		GetByID(gomock.Any(), "u1").
		Return(model.User{ID: "u1", Username: "alice"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.h.Me(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"id":"u1"`)
}

func (s *AuthHandlerSuite) TestMe_NotFound() {
	ctx := context.WithValue(context.Background(), middleware.ContextUserID, "u1")
	s.userSvc.EXPECT().
		GetByID(gomock.Any(), "u1").
		Return(model.User{}, apperr.ErrNotFound)

	req := httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.h.Me(rec, req)
	s.Equal(http.StatusNotFound, rec.Code)
}

func (s *AuthHandlerSuite) TestListUsers_Success() {
	s.userSvc.EXPECT().
		ListActive(gomock.Any()).
		Return([]model.User{{ID: "u1"}, {ID: "u2"}}, nil)

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	s.h.ListUsers(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"u1"`)
	s.Contains(rec.Body.String(), `"u2"`)
}

func (s *AuthHandlerSuite) TestListUsers_Error() {
	s.userSvc.EXPECT().
		ListActive(gomock.Any()).
		Return(nil, errors.New("boom"))

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	s.h.ListUsers(rec, req)
	s.Equal(http.StatusInternalServerError, rec.Code)
}

func (s *AuthHandlerSuite) TestUpdateMe_Success() {
	ctx := context.WithValue(context.Background(), middleware.ContextUserID, "u1")
	s.userSvc.EXPECT().
		UpdateCurrency(gomock.Any(), "u1", "EUR").
		Return(model.User{ID: "u1", DefaultCurrency: "EUR"}, nil)

	req := httptest.NewRequest(http.MethodPatch, "/me", strings.NewReader(`{"defaultCurrency":"EUR"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.h.UpdateMe(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"defaultCurrency":"EUR"`)
}

func (s *AuthHandlerSuite) TestUpdateMe_InvalidJSON() {
	req := httptest.NewRequest(http.MethodPatch, "/me", strings.NewReader(`{`))
	rec := httptest.NewRecorder()
	s.h.UpdateMe(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *AuthHandlerSuite) TestUpdateMe_ValidationError() {
	ctx := context.WithValue(context.Background(), middleware.ContextUserID, "u1")
	s.userSvc.EXPECT().
		UpdateCurrency(gomock.Any(), "u1", "bad").
		Return(model.User{}, apperr.ErrValidation)

	req := httptest.NewRequest(http.MethodPatch, "/me", strings.NewReader(`{"defaultCurrency":"bad"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.h.UpdateMe(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}
