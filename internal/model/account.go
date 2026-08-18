package model

import "time"

type AccountAccessMode string

const (
	AccountAccessModePersonal AccountAccessMode = "personal"
	AccountAccessModeShared   AccountAccessMode = "shared"
)

func (m AccountAccessMode) IsValid() bool {
	return m == AccountAccessModePersonal || m == AccountAccessModeShared
}

type AccountKind string

const (
	AccountKindSpending   AccountKind = "spending"
	AccountKindDeposit    AccountKind = "deposit"
	AccountKindInvestment AccountKind = "investment"
)

func (k AccountKind) IsValid() bool {
	return k == AccountKindSpending || k == AccountKindDeposit || k == AccountKindInvestment
}

type Account struct {
	ID                 string
	OwnerID            string
	Name               string
	AccessMode         AccountAccessMode
	Kind               AccountKind
	Currency           string
	Icon               *string
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
	AccessMode         AccountAccessMode
	Kind               AccountKind
	Currency           string
	Icon               *string
	InitialBalance     float64
	InitialBalanceDate time.Time
}

type UpdateAccountReq struct {
	Name               *string
	AccessMode         *AccountAccessMode
	Icon               *string
	InitialBalance     *float64
	InitialBalanceDate *time.Time // nil = don't update
}
