package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
)

var (
	jsonResponse = httputil.JSONResponse
	jsonError    = httputil.JSONError
)

type AuthHandler struct {
	auth  *service.AuthService
	users *repository.UserRepository
}

func NewAuthHandler(auth *service.AuthService, users *repository.UserRepository) *AuthHandler {
	return &AuthHandler{auth: auth, users: users}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Username == "" || req.Email == "" || len(req.Password) < 8 {
		jsonError(w, "username, email and password (min 8 chars) are required", http.StatusBadRequest)
		return
	}

	u, err := h.auth.Register(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		jsonError(w, "registration failed: "+err.Error(), http.StatusConflict)
		return
	}
	jsonResponse(w, u, http.StatusCreated)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	u, tokens, err := h.auth.Login(r.Context(), strings.ToLower(req.Email), req.Password)
	if err != nil {
		jsonError(w, err.Error(), http.StatusUnauthorized)
		return
	}
	jsonResponse(w, map[string]any{
		"user":   u,
		"tokens": tokens,
	}, http.StatusOK)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		jsonError(w, "refreshToken is required", http.StatusBadRequest)
		return
	}
	tokens, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		jsonError(w, err.Error(), http.StatusUnauthorized)
		return
	}
	jsonResponse(w, tokens, http.StatusOK)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	u, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}
	jsonResponse(w, u, http.StatusOK)
}

// ListUsers returns id+username+email for all active users (for member picker).
func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.ListActive(r.Context())
	if err != nil {
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// Return only non-sensitive fields
	type userDTO struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	result := make([]userDTO, len(users))
	for i, u := range users {
		result[i] = userDTO{ID: u.ID, Username: u.Username, Email: u.Email}
	}
	jsonResponse(w, result, http.StatusOK)
}

func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DefaultCurrency string `json:"defaultCurrency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.DefaultCurrency = strings.TrimSpace(strings.ToUpper(req.DefaultCurrency))
	if len(req.DefaultCurrency) != 3 {
		jsonError(w, "defaultCurrency must be a 3-letter ISO code", http.StatusBadRequest)
		return
	}

	userID := middleware.UserIDFromCtx(r.Context())
	if err := h.users.UpdateCurrency(r.Context(), userID, req.DefaultCurrency); err != nil {
		jsonError(w, "update failed", http.StatusInternalServerError)
		return
	}

	u, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}
	jsonResponse(w, u, http.StatusOK)
}
