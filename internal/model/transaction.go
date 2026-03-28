package model

import "time"

type TransactionType string

const (
	TransactionTypeExpense  TransactionType = "expense"
	TransactionTypeIncome   TransactionType = "income"
	TransactionTypeTransfer TransactionType = "transfer"
)

func (t TransactionType) IsValid() bool {
	return t == TransactionTypeExpense || t == TransactionTypeIncome || t == TransactionTypeTransfer
}

type Transaction struct {
	ID               string
	AccountID        string
	ToAccountID      *string
	Type             TransactionType
	Amount           float64
	Currency         string
	ExchangeRate     *float64
	CategoryID       *string
	Description      *string
	Date             time.Time
	IncludeInBalance bool
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Shares           []TransactionShare
}

type TransactionShare struct {
	ID            string
	TransactionID string
	UserID        string
	Amount        float64
	IsCustom      bool
}

type CreateTransactionReq struct {
	AccountID        string
	ToAccountID      *string
	Type             TransactionType
	Amount           float64
	Currency         string
	ExchangeRate     *float64
	CategoryID       *string
	Description      *string
	Date             time.Time
	IncludeInBalance bool
	Shares           []ShareReq // nil = auto-calculate from member defaults
}

type UpdateTransactionReq struct {
	Amount           *float64
	CategoryID       *string
	Description      *string
	Date             *time.Time
	IncludeInBalance *bool
	Shares           []ShareReq
}

type ShareReq struct {
	UserID string
	Amount float64
}

type TransactionFilter struct {
	AccountIDs  []string
	CategoryIDs []string
	DateFrom    *time.Time
	DateTo      *time.Time
	Page        int
	Limit       int
}
