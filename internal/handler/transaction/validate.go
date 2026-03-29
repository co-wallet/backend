package transactionhandler

import (
	"errors"
	"strings"
	"time"

	"github.com/co-wallet/backend/internal/model"
)

type shareReq struct {
	UserID string  `json:"userId"`
	Amount float64 `json:"amount"`
}

type createTransactionReq struct {
	AccountID             string                `json:"accountId"`
	ToAccountID           *string               `json:"toAccountId"`
	Type                  model.TransactionType `json:"type"`
	Amount                float64               `json:"amount"`
	Currency              string                `json:"currency"`
	ExchangeRate          *float64              `json:"exchangeRate"`
	DefaultCurrency       *string               `json:"defaultCurrency"`
	DefaultCurrencyAmount *float64              `json:"defaultCurrencyAmount"`
	CategoryID            *string               `json:"categoryId"`
	Description           *string               `json:"description"`
	Date                  time.Time             `json:"date"`
	IncludeInBalance      bool                  `json:"includeInBalance"`
	Shares                []shareReq            `json:"shares"`
	Tags                  []string              `json:"tags"`
}

func (r *createTransactionReq) validate() error {
	if strings.TrimSpace(r.AccountID) == "" {
		return errors.New("accountId is required")
	}
	if !r.Type.IsValid() {
		return errors.New("type must be expense, income or transfer")
	}
	if r.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	if len(r.Currency) != 3 {
		return errors.New("currency must be a 3-letter ISO code")
	}
	if r.Date.IsZero() {
		return errors.New("date is required")
	}
	if r.Type == model.TransactionTypeTransfer && r.ToAccountID == nil {
		return errors.New("toAccountId is required for transfer")
	}
	return nil
}

func (r *createTransactionReq) toModelReq() model.CreateTransactionReq {
	req := model.CreateTransactionReq{
		AccountID:             r.AccountID,
		ToAccountID:           r.ToAccountID,
		Type:                  r.Type,
		Amount:                r.Amount,
		Currency:              strings.ToUpper(r.Currency),
		ExchangeRate:          r.ExchangeRate,
		DefaultCurrency:       r.DefaultCurrency,
		DefaultCurrencyAmount: r.DefaultCurrencyAmount,
		CategoryID:            r.CategoryID,
		Description:           r.Description,
		Date:                  r.Date,
		IncludeInBalance:      r.IncludeInBalance,
		Tags:                  r.Tags,
	}
	if len(r.Shares) > 0 {
		req.Shares = make([]model.ShareReq, len(r.Shares))
		for i, s := range r.Shares {
			req.Shares[i] = model.ShareReq{UserID: s.UserID, Amount: s.Amount}
		}
	}
	return req
}

type updateTransactionReq struct {
	Amount                *float64   `json:"amount"`
	DefaultCurrencyAmount *float64   `json:"defaultCurrencyAmount"`
	CategoryID            *string    `json:"categoryId"`
	Description           *string    `json:"description"`
	Date                  *time.Time `json:"date"`
	IncludeInBalance      *bool      `json:"includeInBalance"`
	Shares                []shareReq `json:"shares"`
	Tags                  []string   `json:"tags"`
}

func (r *updateTransactionReq) validate() error {
	if r.Amount != nil && *r.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	return nil
}

func (r *updateTransactionReq) toModelReq() model.UpdateTransactionReq {
	req := model.UpdateTransactionReq{
		Amount:                r.Amount,
		DefaultCurrencyAmount: r.DefaultCurrencyAmount,
		CategoryID:            r.CategoryID,
		Description:           r.Description,
		Date:                  r.Date,
		IncludeInBalance:      r.IncludeInBalance,
		Tags:                  r.Tags,
	}
	if len(r.Shares) > 0 {
		req.Shares = make([]model.ShareReq, len(r.Shares))
		for i, s := range r.Shares {
			req.Shares[i] = model.ShareReq{UserID: s.UserID, Amount: s.Amount}
		}
	}
	return req
}
