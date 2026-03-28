package transactionhandler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
)

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())

	f := model.TransactionFilter{
		Page:  pageParam(r, 1),
		Limit: limitParam(r, 50),
	}
	if ids := r.URL.Query().Get("account_ids"); ids != "" {
		f.AccountIDs = strings.Split(ids, ",")
	}
	if ids := r.URL.Query().Get("category_ids"); ids != "" {
		f.CategoryIDs = strings.Split(ids, ",")
	}
	if s := r.URL.Query().Get("date_from"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			f.DateFrom = &t
		}
	}
	if s := r.URL.Query().Get("date_to"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			f.DateTo = &t
		}
	}

	txs, err := h.service.List(r.Context(), userID, f)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	resp := make([]TransactionResponse, len(txs))
	for i, tx := range txs {
		resp[i] = toTransactionResponse(tx)
	}
	jsonResponse(w, resp, http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTransactionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := middleware.UserIDFromCtx(r.Context())
	tx, err := h.service.Create(r.Context(), userID, req.toModelReq())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toTransactionResponse(tx), http.StatusCreated)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	txID := chi.URLParam(r, "transactionID")
	userID := middleware.UserIDFromCtx(r.Context())

	tx, err := h.service.GetByID(r.Context(), userID, txID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toTransactionResponse(tx), http.StatusOK)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	txID := chi.URLParam(r, "transactionID")

	var req updateTransactionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.validate(); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := middleware.UserIDFromCtx(r.Context())
	tx, err := h.service.Update(r.Context(), userID, txID, req.toModelReq())
	if err != nil {
		handleServiceError(w, err)
		return
	}
	jsonResponse(w, toTransactionResponse(tx), http.StatusOK)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	txID := chi.URLParam(r, "transactionID")
	userID := middleware.UserIDFromCtx(r.Context())

	if err := h.service.Delete(r.Context(), userID, txID); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pageParam(r *http.Request, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		return v
	}
	return def
}

func limitParam(r *http.Request, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		return v
	}
	return def
}
