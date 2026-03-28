package accounthandler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/middleware"
)

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	members, err := h.service.GetMembers(r.Context(), accountID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toMemberResponses(members), http.StatusOK)
}

func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")

	var req addMemberReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	members, err := h.service.AddMember(r.Context(), accountID, req.Username, req.DefaultShare)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toMemberResponses(members), http.StatusOK)
}

func (h *Handler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	memberUserID := chi.URLParam(r, "userID")

	var req updateMemberReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	members, err := h.service.UpdateMember(r.Context(), accountID, memberUserID, req.DefaultShare)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toMemberResponses(members), http.StatusOK)
}

func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	memberUserID := chi.URLParam(r, "userID")
	requesterID := middleware.UserIDFromCtx(r.Context())

	if err := h.service.RemoveMember(r.Context(), requesterID, accountID, memberUserID); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
