package model

import "time"

type AccountType string

const (
	AccountTypePersonal AccountType = "personal"
	AccountTypeShared   AccountType = "shared"
)

type Account struct {
	ID                 string
	OwnerID            string
	Name               string
	Type               AccountType
	Currency           string
	Icon               *string
	IncludeInBalance   bool
	InitialBalance     float64
	InitialBalanceDate time.Time
	DeletedAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// Populated on demand
	Members []AccountMember
}

type AccountMember struct {
	AccountID    string
	UserID       string
	Username     string
	DefaultShare float64
}

// AccountBalance holds computed balance fields for one account.
type AccountBalance struct {
	AccountID      string
	BalanceNative  float64 // user's share in account's native currency
	BalanceDisplay float64 // user's share in display currency
	TotalNative    float64 // all-member total in account's native currency
	TotalDisplay   float64 // all-member total in display currency
}

// Service-level DTOs

type CreateAccountReq struct {
	Name               string
	Type               AccountType
	Currency           string
	Icon               *string
	IncludeInBalance   bool
	InitialBalance     float64
	InitialBalanceDate time.Time
}

type UpdateAccountReq struct {
	Name               *string
	Icon               *string
	IncludeInBalance   *bool
	InitialBalance     *float64
	InitialBalanceDate *time.Time // nil = don't update
}
