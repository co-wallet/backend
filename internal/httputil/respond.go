package httputil

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/co-wallet/backend/internal/apperr"
)

func JSONResponse(w http.ResponseWriter, data any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func JSONError(w http.ResponseWriter, message string, status int) {
	JSONResponse(w, map[string]string{"error": message}, status)
}

func HandleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apperr.ErrNotFound):
		JSONError(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, apperr.ErrForbidden):
		JSONError(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, apperr.ErrValidation):
		JSONError(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, apperr.ErrConflict):
		JSONError(w, err.Error(), http.StatusConflict)
	case errors.Is(err, apperr.ErrUnauthorized):
		JSONError(w, err.Error(), http.StatusUnauthorized)
	default:
		JSONError(w, "internal server error", http.StatusInternalServerError)
	}
}
