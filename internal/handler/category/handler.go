package categoryhandler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/service"
)

type Handler struct {
	service *service.CategoryService
}

func New(svc *service.CategoryService) *Handler {
	return &Handler{service: svc}
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrNotFound):
		jsonError(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, apperr.ErrForbidden):
		jsonError(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, apperr.ErrValidation):
		jsonError(w, err.Error(), http.StatusBadRequest)
	default:
		jsonError(w, "internal server error", http.StatusInternalServerError)
	}
}

func jsonResponse(w http.ResponseWriter, data any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, message string, status int) {
	jsonResponse(w, map[string]string{"error": message}, status)
}
