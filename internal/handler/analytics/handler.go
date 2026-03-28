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

type Handler struct {
	svc analyticsService
}

func New(svc analyticsService) *Handler {
	return &Handler{svc: svc}
}

func parseFilter(r *http.Request, userID string) model.AnalyticsFilter {
	q := r.URL.Query()

	dateFrom := q.Get("date_from")
	dateTo := q.Get("date_to")

	// Default: current month
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

	return model.AnalyticsFilter{
		UserID:          userID,
		DateFrom:        dateFrom,
		DateTo:          dateTo,
		AccountIDs:      accountIDs,
		DisplayCurrency: q.Get("currency"),
	}
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	f := parseFilter(r, userID)

	summary, err := h.svc.Summary(r.Context(), f)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, summary, http.StatusOK)
}

func (h *Handler) ByCategory(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	f := parseFilter(r, userID)

	stats, err := h.svc.ByCategory(r.Context(), f)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, stats, http.StatusOK)
}

func (h *Handler) ByTag(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	f := parseFilter(r, userID)

	stats, err := h.svc.ByTag(r.Context(), f)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, stats, http.StatusOK)
}
