package taghandler_test

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
	taghandler "github.com/co-wallet/backend/internal/handler/tag"
	"github.com/co-wallet/backend/internal/handler/tag/mocks"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
)

type TagHandlerSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	svc  *mocks.MocktagService
	h    *taghandler.Handler
}

func TestTagHandlerSuite(t *testing.T) {
	suite.Run(t, new(TagHandlerSuite))
}

func (s *TagHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.svc = mocks.NewMocktagService(s.ctrl)
	s.h = taghandler.New(s.svc)
}

func withUser(req *http.Request, id string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.ContextUserID, id))
}

func withTagParam(req *http.Request, id string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("tagID", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
}

func (s *TagHandlerSuite) TestList_PassesQuery() {
	s.svc.EXPECT().
		List(gomock.Any(), "u1", "food").
		Return([]model.TagWithCount{{Tag: model.Tag{ID: "t1", Name: "food"}, TxCount: 3}}, nil)

	req := withUser(httptest.NewRequest(http.MethodGet, "/tags?q=food", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.List(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"food"`)
}

func (s *TagHandlerSuite) TestRename_Success() {
	s.svc.EXPECT().
		Rename(gomock.Any(), "u1", "t1", "new").
		Return(model.Tag{ID: "t1", Name: "new"}, nil)

	body := `{"name":"new"}`
	req := withTagParam(withUser(httptest.NewRequest(http.MethodPatch, "/tags/t1", strings.NewReader(body)), "u1"), "t1")
	rec := httptest.NewRecorder()
	s.h.Rename(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"new"`)
}

func (s *TagHandlerSuite) TestRename_EmptyName() {
	body := `{"name":"  "}`
	req := withTagParam(withUser(httptest.NewRequest(http.MethodPatch, "/tags/t1", strings.NewReader(body)), "u1"), "t1")
	rec := httptest.NewRecorder()
	s.h.Rename(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *TagHandlerSuite) TestRename_Conflict() {
	s.svc.EXPECT().
		Rename(gomock.Any(), "u1", "t1", "dup").
		Return(model.Tag{}, apperr.ErrConflict)

	body := `{"name":"dup"}`
	req := withTagParam(withUser(httptest.NewRequest(http.MethodPatch, "/tags/t1", strings.NewReader(body)), "u1"), "t1")
	rec := httptest.NewRecorder()
	s.h.Rename(rec, req)
	s.Equal(http.StatusConflict, rec.Code)
}

func (s *TagHandlerSuite) TestDelete_Success() {
	s.svc.EXPECT().
		Delete(gomock.Any(), "u1", "t1").
		Return(nil)

	req := withTagParam(withUser(httptest.NewRequest(http.MethodDelete, "/tags/t1", nil), "u1"), "t1")
	rec := httptest.NewRecorder()
	s.h.Delete(rec, req)
	s.Equal(http.StatusNoContent, rec.Code)
}

func (s *TagHandlerSuite) TestDelete_NotFound() {
	s.svc.EXPECT().
		Delete(gomock.Any(), "u1", "t1").
		Return(apperr.ErrNotFound)

	req := withTagParam(withUser(httptest.NewRequest(http.MethodDelete, "/tags/t1", nil), "u1"), "t1")
	rec := httptest.NewRecorder()
	s.h.Delete(rec, req)
	s.Equal(http.StatusNotFound, rec.Code)
}
