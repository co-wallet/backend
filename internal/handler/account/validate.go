package accounthandler

import (
	"fmt"
	"strings"

	"github.com/co-wallet/backend/internal/model"
)

type createAccountReq struct {
	Name               string              `json:"name"`
	Type               model.AccountType   `json:"type"`
	Currency           string              `json:"currency"`
	Icon               *string             `json:"icon"`
	IncludeInBalance   *bool               `json:"includeInBalance"`
	InitialBalance     float64             `json:"initialBalance"`
	InitialBalanceDate *string             `json:"initialBalanceDate"`
}

func (r *createAccountReq) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))

	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(r.Currency) != 3 {
		return fmt.Errorf("currency must be a 3-letter ISO code")
	}
	if r.Type != model.AccountTypePersonal && r.Type != model.AccountTypeShared {
		return fmt.Errorf("type must be 'personal' or 'shared'")
	}
	return nil
}

func (r *createAccountReq) toModelReq() model.CreateAccountReq {
	includeInBalance := true
	if r.IncludeInBalance != nil {
		includeInBalance = *r.IncludeInBalance
	}
	return model.CreateAccountReq{
		Name:               r.Name,
		Type:               r.Type,
		Currency:           r.Currency,
		Icon:               r.Icon,
		IncludeInBalance:   includeInBalance,
		InitialBalance:     r.InitialBalance,
		InitialBalanceDate: r.InitialBalanceDate,
	}
}

type updateAccountReq struct {
	Name             *string `json:"name"`
	Icon             *string `json:"icon"`
	IncludeInBalance *bool   `json:"includeInBalance"`
}

func (r *updateAccountReq) validate() error {
	if r.Name != nil {
		*r.Name = strings.TrimSpace(*r.Name)
		if *r.Name == "" {
			return fmt.Errorf("name cannot be empty")
		}
	}
	return nil
}

type addMemberReq struct {
	Username     string  `json:"username"`
	DefaultShare float64 `json:"defaultShare"`
}

func (r *addMemberReq) validate() error {
	r.Username = strings.TrimSpace(r.Username)
	if r.Username == "" {
		return fmt.Errorf("username is required")
	}
	if r.DefaultShare < 0 || r.DefaultShare > 1 {
		return fmt.Errorf("defaultShare must be between 0 and 1")
	}
	return nil
}

type updateMemberReq struct {
	DefaultShare float64 `json:"defaultShare"`
}

func (r *updateMemberReq) validate() error {
	if r.DefaultShare < 0 || r.DefaultShare > 1 {
		return fmt.Errorf("defaultShare must be between 0 and 1")
	}
	return nil
}
