package categoryhandler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
)

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	catType := model.CategoryType(r.URL.Query().Get("type"))
	if catType != model.CategoryTypeExpense && catType != model.CategoryTypeIncome {
		jsonError(w, "type must be 'expense' or 'income'", http.StatusBadRequest)
		return
	}

	tree, err := h.service.List(r.Context(), userID, catType)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toCategoryNodeResponses(tree), http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createCategoryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := middleware.UserIDFromCtx(r.Context())
	cat, err := h.service.Create(r.Context(), userID, req.toModelReq())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toCategoryResponse(cat), http.StatusCreated)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	categoryID := chi.URLParam(r, "categoryID")

	var req updateCategoryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := middleware.UserIDFromCtx(r.Context())
	cat, err := h.service.Update(r.Context(), userID, categoryID, req.toModelReq())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toCategoryResponse(cat), http.StatusOK)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	categoryID := chi.URLParam(r, "categoryID")
	userID := middleware.UserIDFromCtx(r.Context())

	if err := h.service.Delete(r.Context(), userID, categoryID); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
