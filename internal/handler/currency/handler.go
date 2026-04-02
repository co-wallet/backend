package currencyhandler

import (
	"context"
	"net/http"
	"strings"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/model"
)

type currencyService interface {
	ListActive(ctx context.Context, extraCodes []string) ([]model.CurrencyWithRate, error)
}

type Handler struct {
	svc currencyService
}

func New(svc currencyService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	var extraCodes []string
	if codes := r.URL.Query().Get("codes"); codes != "" {
		for _, c := range strings.Split(codes, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				extraCodes = append(extraCodes, c)
			}
		}
	}
	currencies, err := h.svc.ListActive(r.Context(), extraCodes)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, currencies, http.StatusOK)
}
