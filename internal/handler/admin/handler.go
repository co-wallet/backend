package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type adminService interface {
	ListUsers(ctx context.Context) ([]model.User, error)
	GetUser(ctx context.Context, id string) (model.User, error)
	UpdateUser(ctx context.Context, id string, req service.AdminUpdateUserReq) error
	ListAllCurrencies(ctx context.Context) ([]model.CurrencyWithRate, error)
	CreateCurrency(ctx context.Context, req service.CreateCurrencyReq) error
	UpdateCurrency(ctx context.Context, code string, req service.UpdateCurrencyReq) error
	RefreshRates(ctx context.Context) error
}

type Handler struct {
	svc adminService
}

func New(svc adminService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListUsers(r.Context())
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, users, http.StatusOK)
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userID")
	user, err := h.svc.GetUser(r.Context(), id)
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, user, http.StatusOK)
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userID")
	var req service.AdminUpdateUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := h.svc.UpdateUser(r.Context(), id, req); err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListCurrencies(w http.ResponseWriter, r *http.Request) {
	currencies, err := h.svc.ListAllCurrencies(r.Context())
	if err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	httputil.JSONResponse(w, currencies, http.StatusOK)
}

func (h *Handler) CreateCurrency(w http.ResponseWriter, r *http.Request) {
	var req service.CreateCurrencyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := h.svc.CreateCurrency(r.Context(), req); err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) UpdateCurrency(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	var req service.UpdateCurrencyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.JSONError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := h.svc.UpdateCurrency(r.Context(), code, req); err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RefreshRates(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RefreshRates(r.Context()); err != nil {
		httputil.HandleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
