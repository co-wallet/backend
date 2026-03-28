package model

import "time"

type AccountType string

const (
	AccountTypePersonal AccountType = "personal"
	AccountTypeShared   AccountType = "shared"
)

type Account struct {
	ID                 string      `json:"id"`
	OwnerID            string      `json:"ownerId"`
	Name               string      `json:"name"`
	Type               AccountType `json:"type"`
	Currency           string      `json:"currency"`
	Icon               *string     `json:"icon"`
	IncludeInBalance   bool        `json:"includeInBalance"`
	InitialBalance     float64     `json:"initialBalance"`
	InitialBalanceDate *string     `json:"initialBalanceDate"`
	DeletedAt          *time.Time  `json:"-"`
	CreatedAt          time.Time   `json:"createdAt"`
	UpdatedAt          time.Time   `json:"updatedAt"`

	// Populated on demand
	Members []AccountMember `json:"members,omitempty"`
}

type AccountMember struct {
	AccountID    string  `json:"accountId"`
	UserID       string  `json:"userId"`
	Username     string  `json:"username,omitempty"`
	DefaultShare float64 `json:"defaultShare"`
}

// Request DTOs

type CreateAccountReq struct {
	Name               string      `json:"name"`
	Type               AccountType `json:"type"`
	Currency           string      `json:"currency"`
	Icon               *string     `json:"icon"`
	IncludeInBalance   bool        `json:"includeInBalance"`
	InitialBalance     float64     `json:"initialBalance"`
	InitialBalanceDate *string     `json:"initialBalanceDate"`
}

type UpdateAccountReq struct {
	Name             *string `json:"name"`
	Icon             *string `json:"icon"`
	IncludeInBalance *bool   `json:"includeInBalance"`
}
