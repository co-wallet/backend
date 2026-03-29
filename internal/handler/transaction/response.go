package transactionhandler

import (
	"time"

	"github.com/co-wallet/backend/internal/model"
)

type ShareResponse struct {
	UserID   string  `json:"userId"`
	Amount   float64 `json:"amount"`
	IsCustom bool    `json:"isCustom"`
}

type TagResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TransactionResponse struct {
	ID                    string          `json:"id"`
	AccountID             string          `json:"accountId"`
	ToAccountID           *string         `json:"toAccountId"`
	Type                  string          `json:"type"`
	Amount                float64         `json:"amount"`
	Currency              string          `json:"currency"`
	ExchangeRate          *float64        `json:"exchangeRate"`
	DefaultCurrency       *string         `json:"defaultCurrency"`
	DefaultCurrencyAmount *float64        `json:"defaultCurrencyAmount"`
	CategoryID            *string         `json:"categoryId"`
	Description           *string         `json:"description"`
	Date                  time.Time       `json:"date"`
	IncludeInBalance      bool            `json:"includeInBalance"`
	CreatedBy             string          `json:"createdBy"`
	CreatedAt             time.Time       `json:"createdAt"`
	Shares                []ShareResponse `json:"shares"`
	Tags                  []TagResponse   `json:"tags"`
}

func toTransactionResponse(tx model.Transaction) TransactionResponse {
	shares := make([]ShareResponse, len(tx.Shares))
	for i, s := range tx.Shares {
		shares[i] = ShareResponse{UserID: s.UserID, Amount: s.Amount, IsCustom: s.IsCustom}
	}
	tags := make([]TagResponse, len(tx.Tags))
	for i, t := range tx.Tags {
		tags[i] = TagResponse{ID: t.ID, Name: t.Name}
	}
	return TransactionResponse{
		ID:                    tx.ID,
		AccountID:             tx.AccountID,
		ToAccountID:           tx.ToAccountID,
		Type:                  string(tx.Type),
		Amount:                tx.Amount,
		Currency:              tx.Currency,
		ExchangeRate:          tx.ExchangeRate,
		DefaultCurrency:       tx.DefaultCurrency,
		DefaultCurrencyAmount: tx.DefaultCurrencyAmount,
		CategoryID:            tx.CategoryID,
		Description:           tx.Description,
		Date:                  tx.Date,
		IncludeInBalance:      tx.IncludeInBalance,
		CreatedBy:             tx.CreatedBy,
		CreatedAt:             tx.CreatedAt,
		Shares:                shares,
		Tags:                  tags,
	}
}
