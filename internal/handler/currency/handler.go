package currencyhandler

import (
	"context"
	"net/http"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/model"
)

type currencyService interface {
	ListActive(ctx context.Context) ([]model.CurrencyWithRate, error)
}

type Handler struct {
	svc currencyService
}

func New(svc currencyService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	currencies, err := h.svc.ListActive(r.Context())
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, currencies, http.StatusOK)
}
