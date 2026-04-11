package categoryhandler_test

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
	categoryhandler "github.com/co-wallet/backend/internal/handler/category"
	"github.com/co-wallet/backend/internal/handler/category/mocks"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type CategoryHandlerSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	svc  *mocks.MockcategoryService
	h    *categoryhandler.Handler
}

func TestCategoryHandlerSuite(t *testing.T) {
	suite.Run(t, new(CategoryHandlerSuite))
}

func (s *CategoryHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.svc = mocks.NewMockcategoryService(s.ctrl)
	s.h = categoryhandler.New(s.svc)
}

func withUser(req *http.Request, id string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.ContextUserID, id))
}

func withCategoryParam(req *http.Request, id string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("categoryID", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
}

func (s *CategoryHandlerSuite) TestList_Success() {
	s.svc.EXPECT().
		List(gomock.Any(), "u1", model.CategoryTypeExpense).
		Return([]service.CategoryNode{{Category: model.Category{ID: "c1", Name: "Food"}}}, nil)

	req := withUser(httptest.NewRequest(http.MethodGet, "/categories?type=expense", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.List(rec, req)
	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), `"Food"`)
}

func (s *CategoryHandlerSuite) TestList_InvalidType() {
	req := withUser(httptest.NewRequest(http.MethodGet, "/categories?type=bogus", nil), "u1")
	rec := httptest.NewRecorder()
	s.h.List(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *CategoryHandlerSuite) TestCreate_Success() {
	s.svc.EXPECT().
		Create(gomock.Any(), "u1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, req model.CreateCategoryReq) (model.Category, error) {
			s.Equal("Food", req.Name)
			s.Equal(model.CategoryTypeExpense, req.Type)
			return model.Category{ID: "c1", Name: "Food", Type: model.CategoryTypeExpense}, nil
		})

	body := `{"name":"Food","type":"expense"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusCreated, rec.Code)
}

func (s *CategoryHandlerSuite) TestCreate_ValidationError() {
	body := `{"name":"","type":"expense"}`
	req := withUser(httptest.NewRequest(http.MethodPost, "/categories", strings.NewReader(body)), "u1")
	rec := httptest.NewRecorder()
	s.h.Create(rec, req)
	s.Equal(http.StatusBadRequest, rec.Code)
}

func (s *CategoryHandlerSuite) TestUpdate_Success() {
	s.svc.EXPECT().
		Update(gomock.Any(), "u1", "c1", gomock.Any()).
		Return(model.Category{ID: "c1", Name: "Groceries"}, nil)

	body := `{"name":"Groceries"}`
	req := withCategoryParam(withUser(httptest.NewRequest(http.MethodPatch, "/categories/c1", strings.NewReader(body)), "u1"), "c1")
	rec := httptest.NewRecorder()
	s.h.Update(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *CategoryHandlerSuite) TestUpdate_Forbidden() {
	s.svc.EXPECT().
		Update(gomock.Any(), "u1", "c1", gomock.Any()).
		Return(model.Category{}, apperr.ErrForbidden)

	body := `{"name":"X"}`
	req := withCategoryParam(withUser(httptest.NewRequest(http.MethodPatch, "/categories/c1", strings.NewReader(body)), "u1"), "c1")
	rec := httptest.NewRecorder()
	s.h.Update(rec, req)
	s.Equal(http.StatusForbidden, rec.Code)
}

func (s *CategoryHandlerSuite) TestDelete_Success() {
	s.svc.EXPECT().
		Delete(gomock.Any(), "u1", "c1").
		Return(nil)

	req := withCategoryParam(withUser(httptest.NewRequest(http.MethodDelete, "/categories/c1", nil), "u1"), "c1")
	rec := httptest.NewRecorder()
	s.h.Delete(rec, req)
	s.Equal(http.StatusNoContent, rec.Code)
}

func (s *CategoryHandlerSuite) TestDelete_Conflict() {
	s.svc.EXPECT().
		Delete(gomock.Any(), "u1", "c1").
		Return(apperr.ErrConflict)

	req := withCategoryParam(withUser(httptest.NewRequest(http.MethodDelete, "/categories/c1", nil), "u1"), "c1")
	rec := httptest.NewRecorder()
	s.h.Delete(rec, req)
	s.Equal(http.StatusConflict, rec.Code)
}
