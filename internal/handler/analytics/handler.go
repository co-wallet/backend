package analytics

import (
	"context"
	"net/http"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
)

type analyticsService interface {
	Summary(ctx context.Context, f model.AnalyticsFilter) (model.AnalyticsSummary, error)
	ByCategory(ctx context.Context, f model.AnalyticsFilter) ([]model.CategoryStat, error)
	ByTag(ctx context.Context, f model.AnalyticsFilter) ([]model.TagStat, error)
}

type userSource interface {
	GetByID(ctx context.Context, id string) (model.User, error)
}

type Handler struct {
	svc   analyticsService
	users userSource
}

func New(svc analyticsService, users userSource) *Handler {
	return &Handler{svc: svc, users: users}
}

var (
	jsonResponse       = httputil.JSONResponse
	jsonError          = httputil.JSONError
	handleServiceError = httputil.HandleServiceError
)

func (h *Handler) resolveFilter(r *http.Request, userID string) (model.AnalyticsFilter, error) {
	p, err := parseFilterParams(r.URL.Query())
	if err != nil {
		return model.AnalyticsFilter{}, err
	}
	defaultCurrency := ""
	if p.Currency == "" {
		if u, err := h.users.GetByID(r.Context(), userID); err == nil {
			defaultCurrency = u.DefaultCurrency
		}
	}
	return p.toFilter(userID, defaultCurrency), nil
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	f, err := h.resolveFilter(r, userID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	summary, err := h.svc.Summary(r.Context(), f)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toSummaryResponse(summary), http.StatusOK)
}

func (h *Handler) ByCategory(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	f, err := h.resolveFilter(r, userID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	stats, err := h.svc.ByCategory(r.Context(), f)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toCategoryStatResponses(stats), http.StatusOK)
}

func (h *Handler) ByTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	f, err := h.resolveFilter(r, userID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	stats, err := h.svc.ByTag(r.Context(), f)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toTagStatResponses(stats), http.StatusOK)
}
