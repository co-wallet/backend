package analytics

import (
	"context"
	"net/http"
	"strings"
	"time"

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

func (h *Handler) parseFilter(r *http.Request, userID string) model.AnalyticsFilter {
	q := r.URL.Query()

	dateFrom := q.Get("date_from")
	dateTo := q.Get("date_to")

	now := time.Now()
	if dateFrom == "" {
		dateFrom = now.Format("2006-01") + "-01"
	}
	if dateTo == "" {
		dateTo = now.Format("2006-01-02")
	}

	var accountIDs []string
	if raw := q.Get("account_ids"); raw != "" {
		for _, id := range strings.Split(raw, ",") {
			if id = strings.TrimSpace(id); id != "" {
				accountIDs = append(accountIDs, id)
			}
		}
	}

	currency := q.Get("currency")
	if currency == "" {
		if u, err := h.users.GetByID(r.Context(), userID); err == nil {
			currency = u.DefaultCurrency
		}
	}

	return model.AnalyticsFilter{
		UserID:          userID,
		DateFrom:        dateFrom,
		DateTo:          dateTo,
		AccountIDs:      accountIDs,
		DisplayCurrency: currency,
		TxType:          q.Get("type"),
	}
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	f := h.parseFilter(r, userID)

	summary, err := h.svc.Summary(r.Context(), f)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, summary, http.StatusOK)
}

func (h *Handler) ByCategory(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	f := h.parseFilter(r, userID)

	stats, err := h.svc.ByCategory(r.Context(), f)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, stats, http.StatusOK)
}

func (h *Handler) ByTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	f := h.parseFilter(r, userID)

	stats, err := h.svc.ByTag(r.Context(), f)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, stats, http.StatusOK)
}
