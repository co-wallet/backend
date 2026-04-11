package invite_test

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
	"github.com/co-wallet/backend/internal/handler/invite"
	"github.com/co-wallet/backend/internal/handler/invite/mocks"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type InviteHandlerSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	svc  *mocks.MockinviteService
	h    *invite.Handler
}

func TestInviteHandlerSuite(t *testing.T) {
	suite.Run(t, new(InviteHandlerSuite))
}

func (s *InviteHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.svc = mocks.NewMockinviteService(s.ctrl)
	s.h = invite.New(s.svc)
}

func withURLParam(req *http.Request, key, val string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, val)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
}

func (s *InviteHandlerSuite) TestCreate_Success() {
	ctx := context.WithValue(context.Background(), middleware.ContextUserID, "admin-1")
	s.svc.EXPECT().
		CreateInvite(gomock.Any(), "x@y.z", "admin-1").
		Return(model.Invite{ID: "inv-1", Email: "x@y.z"}, "https://host/invite/tok", nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/invites", strings.NewReader(`{"email":"x@y.z"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusCreated, rec.Code)
	s.Contains(rec.Body.String(), `"inviteUrl":"https://host/invite/tok"`)
	s.Contains(rec.Body.String(), `"id":"inv-1"`)
}

func (s *InviteHandlerSuite) TestCreate_MissingEmail() {
	req := httptest.NewRequest(http.MethodPost, "/admin/invites", strings.NewReader(`{"email":""}`))
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *InviteHandlerSuite) TestCreate_InvalidJSON() {
	req := httptest.NewRequest(http.MethodPost, "/admin/invites", strings.NewReader(`nope`))
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *InviteHandlerSuite) TestList_Success() {
	s.svc.EXPECT().
		ListInvites(gomock.Any()).
		Return([]model.Invite{{ID: "i1"}, {ID: "i2"}}, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/invites", nil)
	rec := httptest.NewRecorder()
	s.h.List(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"i1"`)
}

func (s *InviteHandlerSuite) TestValidate_Success() {
	s.svc.EXPECT().
		ValidateToken(gomock.Any(), "tok").
		Return(&model.Invite{Email: "x@y.z"}, nil)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/invites/tok", nil), "token", "tok")
	rec := httptest.NewRecorder()
	s.h.Validate(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"email":"x@y.z"`)
}

func (s *InviteHandlerSuite) TestValidate_Expired() {
	s.svc.EXPECT().
		ValidateToken(gomock.Any(), "tok").
		Return(nil, apperr.ErrNotFound)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/invites/tok", nil), "token", "tok")
	rec := httptest.NewRecorder()
	s.h.Validate(rec, req)
	s.Equal(http.StatusNotFound, rec.Code)
}

func (s *InviteHandlerSuite) TestAccept_Success() {
	s.svc.EXPECT().
		AcceptInvite(gomock.Any(), gomock.AssignableToTypeOf(service.AcceptInviteReq{})).
		DoAndReturn(func(_ context.Context, r service.AcceptInviteReq) (model.User, service.TokenPair, error) {
			s.Equal("tok", r.Token)
			s.Equal("alice", r.Username)
			s.Equal("pw", r.Password)
			s.Equal("USD", r.DefaultCurrency)
			return model.User{ID: "u1"}, service.TokenPair{AccessToken: "a", RefreshToken: "r"}, nil
		})

	body := `{"username":"alice","password":"pw","defaultCurrency":"USD"}`
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/invites/tok/accept", strings.NewReader(body)), "token", "tok")
	rec := httptest.NewRecorder()
	s.h.Accept(rec, req)
	s.Equal(http.StatusCreated, rec.Code)
	s.Contains(rec.Body.String(), `"id":"u1"`)
	s.Contains(rec.Body.String(), `"accessToken":"a"`)
}

func (s *InviteHandlerSuite) TestAccept_InvalidJSON() {
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/invites/tok/accept", strings.NewReader(`{`)), "token", "tok")
	rec := httptest.NewRecorder()
	s.h.Accept(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *InviteHandlerSuite) TestAccept_Conflict() {
	s.svc.EXPECT().
		AcceptInvite(gomock.Any(), gomock.Any()).
		Return(model.User{}, service.TokenPair{}, apperr.ErrConflict)

	body := `{"username":"alice","password":"pw","defaultCurrency":"USD"}`
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/invites/tok/accept", strings.NewReader(body)), "token", "tok")
	rec := httptest.NewRecorder()
	s.h.Accept(rec, req)
	s.Equal(http.StatusConflict, rec.Code)
}
