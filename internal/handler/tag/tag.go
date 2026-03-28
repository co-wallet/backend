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
	resp := make([]tagResponse, len(tags))
	for i, t := range tags {
		resp[i] = tagResponse{ID: t.ID, Name: t.Name, TxCount: t.TxCount}
	}
	jsonResponse(w, resp, http.StatusOK)
}

func (h *Handler) Rename(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "tagID")
	userID := middleware.UserIDFromCtx(r.Context())

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}

	t, err := h.service.Rename(r.Context(), userID, tagID, req.Name)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, tagResponse{ID: t.ID, Name: t.Name}, http.StatusOK)
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

type tagResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	TxCount int    `json:"txCount,omitempty"`
}
