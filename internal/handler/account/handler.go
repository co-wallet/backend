package accounthandler

import (
	"errors"
	"net/http"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/service"
)

type Handler struct {
	service *service.AccountService
}

func New(svc *service.AccountService) *Handler {
	return &Handler{service: svc}
}

// handleServiceError maps typed service errors to HTTP responses.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrNotFound):
		jsonError(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, apperr.ErrForbidden):
		jsonError(w, err.Error(), http.StatusForbidden)
	default:
		jsonError(w, "internal server error", http.StatusInternalServerError)
	}
}
