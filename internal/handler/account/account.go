package accounthandler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
)

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	accounts, err := h.accounts.ListByUser(r.Context(), userID)
	if err != nil {
		jsonError(w, "failed to list accounts", http.StatusInternalServerError)
		return
	}
	if accounts == nil {
		accounts = []*model.Account{}
	}
	jsonResponse(w, accounts, http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAccountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := middleware.UserIDFromCtx(r.Context())
	a, err := h.service.CreateAccount(r.Context(), userID, req.toModelReq())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, a, http.StatusCreated)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	a, err := h.accounts.GetByID(r.Context(), accountID)
	if err != nil || a.DeletedAt != nil {
		jsonError(w, "account not found", http.StatusNotFound)
		return
	}
	if a.Type == model.AccountTypeShared {
		members, err := h.accounts.GetMembers(r.Context(), accountID)
		if err != nil {
			jsonError(w, "failed to load members", http.StatusInternalServerError)
			return
		}
		a.Members = members
	}
	jsonResponse(w, a, http.StatusOK)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	a, err := h.accounts.GetByID(r.Context(), accountID)
	if err != nil || a.DeletedAt != nil {
		jsonError(w, "account not found", http.StatusNotFound)
		return
	}

	var req updateAccountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name != nil {
		a.Name = strings.TrimSpace(*req.Name)
	}
	if req.Icon != nil {
		a.Icon = req.Icon
	}
	if req.IncludeInBalance != nil {
		a.IncludeInBalance = *req.IncludeInBalance
	}

	if err := h.accounts.Update(r.Context(), a); err != nil {
		jsonError(w, "failed to update account", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, a, http.StatusOK)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	requesterID := middleware.UserIDFromCtx(r.Context())

	if err := h.service.DeleteAccount(r.Context(), requesterID, accountID); err != nil {
		switch err.Error() {
		case "account not found":
			jsonError(w, err.Error(), http.StatusNotFound)
		case "only the owner can delete an account":
			jsonError(w, err.Error(), http.StatusForbidden)
		default:
			jsonError(w, "failed to delete account", http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
