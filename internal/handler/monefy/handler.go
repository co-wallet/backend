package monefyhandler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
	"github.com/go-chi/chi/v5"
)

//go:generate mockgen -source=handler.go -destination=mocks/mock_import_service.go -package=mocks
type importService interface {
	Availability(context.Context, string) (model.ImportAvailability, error)
	Preview(context.Context, string, io.Reader) (model.ImportPreview, error)
	Configure(context.Context, string, string, map[string]model.AccountKind, map[string]string) (model.ImportPreview, error)
	Confirm(context.Context, string, string, bool) (model.ImportResult, error)
}
type Handler struct{ service importService }

func New(svc importService) *Handler { return &Handler{service: svc} }

func (h *Handler) Availability(w http.ResponseWriter, r *http.Request) {
	a, err := h.service.Availability(r.Context(), middleware.UserIDFromCtx(r.Context()))
	if err != nil {
		respondError(w, err)
		return
	}
	httputil.JSONResponse(w, struct {
		Available bool     `json:"available"`
		Reasons   []string `json:"reasons"`
	}{len(a.Reasons) == 0, a.Reasons}, http.StatusOK)
}
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/octet-stream" {
		httputil.JSONError(w, "expected_sqlite_binary", http.StatusUnsupportedMediaType)
		return
	}
	if r.ContentLength > monefy.MaxFileBytes {
		httputil.JSONError(w, "upload_too_large", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, monefy.MaxFileBytes)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	p, err := h.service.Preview(ctx, middleware.UserIDFromCtx(ctx), r.Body)
	if ctx.Err() != nil {
		respondError(w, ctx.Err())
		return
	}
	if err != nil {
		respondError(w, err)
		return
	}
	httputil.JSONResponse(w, toPreview(p), http.StatusCreated)
}

type optionsRequest struct {
	AccountKinds  map[string]model.AccountKind `json:"account_kinds"`
	CategoryIcons map[string]string            `json:"category_icons"`
}
type confirmRequest struct {
	AcknowledgeExclusions bool `json:"acknowledge_exclusions"`
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return apperr.ErrValidation
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return apperr.ErrValidation
	}
	return nil
}
func (h *Handler) Configure(w http.ResponseWriter, r *http.Request) {
	var req optionsRequest
	if err := decode(w, r, &req); err != nil {
		respondError(w, err)
		return
	}
	p, err := h.service.Configure(r.Context(), middleware.UserIDFromCtx(r.Context()), chi.URLParam(r, "previewID"), req.AccountKinds, req.CategoryIcons)
	if err != nil {
		respondError(w, err)
		return
	}
	httputil.JSONResponse(w, toPreview(p), http.StatusCreated)
}
func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	var req confirmRequest
	if err := decode(w, r, &req); err != nil {
		respondError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := h.service.Confirm(ctx, middleware.UserIDFromCtx(ctx), chi.URLParam(r, "previewID"), req.AcknowledgeExclusions)
	if err != nil {
		respondError(w, err)
		return
	}
	httputil.JSONResponse(w, toResult(result), http.StatusOK)
}
func respondError(w http.ResponseWriter, err error) {
	code := "internal_error"
	status := http.StatusInternalServerError
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr):
		code = "upload_too_large"
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		code = "request_timeout"
		status = http.StatusRequestTimeout
	case errors.Is(err, apperr.ErrUnauthorized):
		code = "unauthorized"
		status = http.StatusUnauthorized
	case errors.Is(err, apperr.ErrNotFound):
		code = "preview_not_found"
		status = http.StatusNotFound
	case errors.Is(err, apperr.ErrValidation):
		code = "invalid_request"
		status = http.StatusBadRequest
	case errors.Is(err, apperr.ErrConflict):
		code = "import_conflict"
		status = http.StatusConflict
	}
	var e *service.ImportError
	if errors.As(err, &e) && status != http.StatusRequestEntityTooLarge && status != http.StatusRequestTimeout {
		code = e.Code
	}
	httputil.JSONError(w, code, status)
}
