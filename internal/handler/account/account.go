package accounthandler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
)

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())

	currency := r.URL.Query().Get("currency")
	if currency == "" {
		if u, err := h.users.GetByID(r.Context(), userID); err == nil {
			currency = u.DefaultCurrency
		}
	}

	accounts, err := h.service.ListByUser(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	balances, err := h.service.ListBalancesByUser(r.Context(), userID, currency)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	resp := make([]AccountResponse, len(accounts))
	for i, a := range accounts {
		resp[i] = toAccountResponse(a)
		if b, ok := balances[a.ID]; ok {
			resp[i].Balance = &BalanceResponse{
				Native:          b.BalanceNative,
				Display:         b.BalanceDisplay,
				TotalNative:     b.TotalNative,
				TotalDisplay:    b.TotalDisplay,
				DisplayCurrency: currency,
			}
		}
	}
	jsonResponse(w, resp, http.StatusOK)
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
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toAccountResponse(a), http.StatusCreated)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	a, err := h.service.GetByID(r.Context(), accountID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if a.Type == model.AccountTypeShared {
		members, err := h.service.GetMembers(r.Context(), accountID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		jsonResponse(w, toAccountResponseWithMembers(a, members), http.StatusOK)
		return
	}
	jsonResponse(w, toAccountResponse(a), http.StatusOK)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")

	var req updateAccountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	updateReq := model.UpdateAccountReq{
		Name:             req.Name,
		Icon:             req.Icon,
		IncludeInBalance: req.IncludeInBalance,
		InitialBalance:   req.InitialBalance,
	}
	if req.InitialBalanceDate != nil {
		t, _ := time.Parse("2006-01-02", *req.InitialBalanceDate)
		updateReq.InitialBalanceDate = &t
	}
	a, err := h.service.UpdateAccount(r.Context(), accountID, updateReq)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toAccountResponse(a), http.StatusOK)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	requesterID := middleware.UserIDFromCtx(r.Context())

	if err := h.service.DeleteAccount(r.Context(), requesterID, accountID); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
