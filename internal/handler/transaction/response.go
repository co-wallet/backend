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

type TransactionResponse struct {
	ID               string          `json:"id"`
	AccountID        string          `json:"accountId"`
	ToAccountID      *string         `json:"toAccountId"`
	Type             string          `json:"type"`
	Amount           float64         `json:"amount"`
	Currency         string          `json:"currency"`
	ExchangeRate     *float64        `json:"exchangeRate"`
	CategoryID       *string         `json:"categoryId"`
	Description      *string         `json:"description"`
	Date             time.Time       `json:"date"`
	IncludeInBalance bool            `json:"includeInBalance"`
	CreatedBy        string          `json:"createdBy"`
	CreatedAt        time.Time       `json:"createdAt"`
	Shares           []ShareResponse `json:"shares"`
}

func toTransactionResponse(tx model.Transaction) TransactionResponse {
	shares := make([]ShareResponse, len(tx.Shares))
	for i, s := range tx.Shares {
		shares[i] = ShareResponse{UserID: s.UserID, Amount: s.Amount, IsCustom: s.IsCustom}
	}
	return TransactionResponse{
		ID:               tx.ID,
		AccountID:        tx.AccountID,
		ToAccountID:      tx.ToAccountID,
		Type:             string(tx.Type),
		Amount:           tx.Amount,
		Currency:         tx.Currency,
		ExchangeRate:     tx.ExchangeRate,
		CategoryID:       tx.CategoryID,
		Description:      tx.Description,
		Date:             tx.Date,
		IncludeInBalance: tx.IncludeInBalance,
		CreatedBy:        tx.CreatedBy,
		CreatedAt:        tx.CreatedAt,
		Shares:           shares,
	}
}
