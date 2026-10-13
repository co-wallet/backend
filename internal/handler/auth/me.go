package auth

import (
	"encoding/json"
	"net/http"

	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/service"
)

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	u, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toUserResponse(u), http.StatusOK)
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.ListActive(r.Context())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	result := make([]PublicUserResponse, len(users))
	for i, u := range users {
		result[i] = toPublicUserResponse(u)
	}
	jsonResponse(w, result, http.StatusOK)
}

func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	var req updateMeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	userID := middleware.UserIDFromCtx(r.Context())
	u, err := h.users.UpdateProfile(r.Context(), userID, service.UpdateProfileReq{
		Username: req.Username, Email: req.Email, DefaultCurrency: req.DefaultCurrency,
		NewPassword: req.NewPassword, CurrentPassword: req.CurrentPassword,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toUserResponse(u), http.StatusOK)
}
