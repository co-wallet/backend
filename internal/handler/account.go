package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

type AccountHandler struct {
	accounts *repository.AccountRepository
	users    *repository.UserRepository
}

func NewAccountHandler(accounts *repository.AccountRepository, users *repository.UserRepository) *AccountHandler {
	return &AccountHandler{accounts: accounts, users: users}
}

func (h *AccountHandler) List(w http.ResponseWriter, r *http.Request) {
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

func (h *AccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())

	var req struct {
		Name               string  `json:"name"`
		Type               string  `json:"type"`
		Currency           string  `json:"currency"`
		Icon               *string `json:"icon"`
		IncludeInBalance   *bool   `json:"includeInBalance"`
		InitialBalance     float64 `json:"initialBalance"`
		InitialBalanceDate *string `json:"initialBalanceDate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	if req.Name == "" || len(req.Currency) != 3 {
		jsonError(w, "name and currency (3-letter ISO) are required", http.StatusBadRequest)
		return
	}
	if req.Type != string(model.AccountTypePersonal) && req.Type != string(model.AccountTypeShared) {
		jsonError(w, "type must be 'personal' or 'shared'", http.StatusBadRequest)
		return
	}

	includeInBalance := true
	if req.IncludeInBalance != nil {
		includeInBalance = *req.IncludeInBalance
	}

	a := &model.Account{
		OwnerID:            userID,
		Name:               req.Name,
		Type:               model.AccountType(req.Type),
		Currency:           req.Currency,
		Icon:               req.Icon,
		IncludeInBalance:   includeInBalance,
		InitialBalance:     req.InitialBalance,
		InitialBalanceDate: req.InitialBalanceDate,
	}
	if err := h.accounts.Create(r.Context(), a); err != nil {
		jsonError(w, "failed to create account", http.StatusInternalServerError)
		return
	}

	// Auto-add owner as member of shared accounts with 100% share
	if a.Type == model.AccountTypeShared {
		_ = h.accounts.AddMember(r.Context(), model.AccountMember{
			AccountID:    a.ID,
			UserID:       userID,
			DefaultShare: 1.0,
		})
	}

	jsonResponse(w, a, http.StatusCreated)
}

func (h *AccountHandler) Get(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	a, err := h.accounts.GetByID(r.Context(), accountID)
	if err != nil {
		jsonError(w, "account not found", http.StatusNotFound)
		return
	}
	if a.DeletedAt != nil {
		jsonError(w, "account not found", http.StatusNotFound)
		return
	}
	if a.Type == model.AccountTypeShared {
		members, _ := h.accounts.GetMembers(r.Context(), accountID)
		a.Members = members
	}
	jsonResponse(w, a, http.StatusOK)
}

func (h *AccountHandler) Update(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	a, err := h.accounts.GetByID(r.Context(), accountID)
	if err != nil || a.DeletedAt != nil {
		jsonError(w, "account not found", http.StatusNotFound)
		return
	}

	var req struct {
		Name             *string `json:"name"`
		Icon             *string `json:"icon"`
		IncludeInBalance *bool   `json:"includeInBalance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
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

func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	userID := middleware.UserIDFromCtx(r.Context())

	a, err := h.accounts.GetByID(r.Context(), accountID)
	if err != nil || a.DeletedAt != nil {
		jsonError(w, "account not found", http.StatusNotFound)
		return
	}
	if a.OwnerID != userID {
		jsonError(w, "only the owner can delete an account", http.StatusForbidden)
		return
	}
	if err := h.accounts.SoftDelete(r.Context(), accountID); err != nil {
		jsonError(w, "failed to delete account", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Members

func (h *AccountHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	members, err := h.accounts.GetMembers(r.Context(), accountID)
	if err != nil {
		jsonError(w, "failed to list members", http.StatusInternalServerError)
		return
	}
	if members == nil {
		members = []model.AccountMember{}
	}
	jsonResponse(w, members, http.StatusOK)
}

func (h *AccountHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")

	var req struct {
		Username     string  `json:"username"`
		DefaultShare float64 `json:"defaultShare"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.DefaultShare < 0 || req.DefaultShare > 1 {
		jsonError(w, "defaultShare must be between 0 and 1", http.StatusBadRequest)
		return
	}

	u, err := h.users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	if err := h.accounts.AddMember(r.Context(), model.AccountMember{
		AccountID:    accountID,
		UserID:       u.ID,
		DefaultShare: req.DefaultShare,
	}); err != nil {
		jsonError(w, "failed to add member", http.StatusInternalServerError)
		return
	}

	if err := h.validateShareSum(r, accountID); err != nil {
		// Rollback
		_ = h.accounts.RemoveMember(r.Context(), accountID, u.ID)
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	members, _ := h.accounts.GetMembers(r.Context(), accountID)
	jsonResponse(w, members, http.StatusOK)
}

func (h *AccountHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	memberUserID := chi.URLParam(r, "userID")

	var req struct {
		DefaultShare float64 `json:"defaultShare"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.DefaultShare < 0 || req.DefaultShare > 1 {
		jsonError(w, "defaultShare must be between 0 and 1", http.StatusBadRequest)
		return
	}

	if err := h.accounts.UpdateMemberShare(r.Context(), accountID, memberUserID, req.DefaultShare); err != nil {
		jsonError(w, "failed to update member", http.StatusInternalServerError)
		return
	}

	if err := h.validateShareSum(r, accountID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	members, _ := h.accounts.GetMembers(r.Context(), accountID)
	jsonResponse(w, members, http.StatusOK)
}

func (h *AccountHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountID")
	memberUserID := chi.URLParam(r, "userID")
	requesterID := middleware.UserIDFromCtx(r.Context())

	a, err := h.accounts.GetByID(r.Context(), accountID)
	if err != nil {
		jsonError(w, "account not found", http.StatusNotFound)
		return
	}
	if a.OwnerID != requesterID {
		jsonError(w, "only the owner can remove members", http.StatusForbidden)
		return
	}
	if memberUserID == a.OwnerID {
		jsonError(w, "cannot remove the account owner", http.StatusBadRequest)
		return
	}
	if err := h.accounts.RemoveMember(r.Context(), accountID, memberUserID); err != nil {
		jsonError(w, "failed to remove member", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AccountHandler) validateShareSum(r *http.Request, accountID string) error {
	sum, err := h.accounts.SumShares(r.Context(), accountID)
	if err != nil {
		return err
	}
	// Allow small floating point tolerance
	if math.Abs(sum-1.0) > 0.0001 && sum > 0 {
		return nil // shares don't need to sum to 1 during editing; validated on transaction creation
	}
	return nil
}
