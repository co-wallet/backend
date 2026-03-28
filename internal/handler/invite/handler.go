package invite

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type inviteService interface {
	CreateInvite(ctx context.Context, email, createdBy string) (model.Invite, string, error)
	ListInvites(ctx context.Context) ([]model.Invite, error)
	ValidateToken(ctx context.Context, token string) (*model.Invite, error)
	AcceptInvite(ctx context.Context, req service.AcceptInviteReq) (*model.User, *service.TokenPair, error)
}

type Handler struct {
	svc inviteService
}

func New(svc inviteService) *Handler {
	return &Handler{svc: svc}
}

// POST /api/admin/invites — create invite (admin only)
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		httputil.JSONError(w, "email is required", http.StatusBadRequest)
		return
	}

	createdBy := middleware.UserIDFromCtx(r.Context())
	inv, inviteURL, err := h.svc.CreateInvite(r.Context(), req.Email, createdBy)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}

	httputil.JSONResponse(w, map[string]any{
		"invite":    inv,
		"inviteUrl": inviteURL,
	}, http.StatusCreated)
}

// GET /api/admin/invites — list all invites (admin only)
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	invites, err := h.svc.ListInvites(r.Context())
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, invites, http.StatusOK)
}

// GET /api/invites/:token — validate token (public)
func (h *Handler) Validate(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	inv, err := h.svc.ValidateToken(r.Context(), token)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	// Return only the email so the frontend can pre-fill it
	httputil.JSONResponse(w, map[string]string{"email": inv.Email}, http.StatusOK)
}

// POST /api/invites/:token/accept — create user account (public)
func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var body struct {
		Username        string `json:"username"`
		Password        string `json:"password"`
		DefaultCurrency string `json:"defaultCurrency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, tokens, err := h.svc.AcceptInvite(r.Context(), service.AcceptInviteReq{
		Token:           token,
		Username:        body.Username,
		Password:        body.Password,
		DefaultCurrency: body.DefaultCurrency,
	})
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}

	httputil.JSONResponse(w, map[string]any{
		"user":   user,
		"tokens": tokens,
	}, http.StatusCreated)
}
