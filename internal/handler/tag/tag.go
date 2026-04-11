package taghandler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/middleware"
)

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	q := r.URL.Query().Get("q")

	tags, err := h.service.List(r.Context(), userID, q)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toTagWithCountResponses(tags), http.StatusOK)
}

func (h *Handler) Rename(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "tagID")
	userID := middleware.UserIDFromCtx(r.Context())

	var req renameTagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	t, err := h.service.Rename(r.Context(), userID, tagID, req.Name)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toTagResponse(t), http.StatusOK)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "tagID")
	userID := middleware.UserIDFromCtx(r.Context())

	if err := h.service.Delete(r.Context(), userID, tagID); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
