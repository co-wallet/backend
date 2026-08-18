package accounthandler

import (
	"fmt"
	"strings"
	"time"

	"github.com/co-wallet/backend/internal/model"
)

type createAccountReq struct {
	Name               string                  `json:"name"`
	AccessMode         model.AccountAccessMode `json:"accessMode"`
	Kind               model.AccountKind       `json:"kind"`
	Currency           string                  `json:"currency"`
	Icon               *string                 `json:"icon"`
	InitialBalance     float64                 `json:"initialBalance"`
	InitialBalanceDate string                  `json:"initialBalanceDate"` // "YYYY-MM-DD" from frontend
}

func (r *createAccountReq) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
	if r.AccessMode == "" {
		r.AccessMode = model.AccountAccessModePersonal
	}
	if r.Kind == "" {
		r.Kind = model.AccountKindSpending
	}

	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(r.Currency) != 3 {
		return fmt.Errorf("currency must be a 3-letter ISO code")
	}
	if !r.AccessMode.IsValid() {
		return fmt.Errorf("accessMode must be 'personal' or 'shared'")
	}
	if !r.Kind.IsValid() {
		return fmt.Errorf("kind must be 'spending', 'deposit', or 'investment'")
	}
	if r.InitialBalanceDate == "" {
		return fmt.Errorf("initialBalanceDate is required")
	}
	if _, err := time.Parse("2006-01-02", r.InitialBalanceDate); err != nil {
		return fmt.Errorf("initialBalanceDate must be YYYY-MM-DD")
	}
	return nil
}

func (r *createAccountReq) toModelReq() model.CreateAccountReq {
	ibd, _ := time.Parse("2006-01-02", r.InitialBalanceDate)
	return model.CreateAccountReq{
		Name:               r.Name,
		AccessMode:         r.AccessMode,
		Kind:               r.Kind,
		Currency:           r.Currency,
		Icon:               r.Icon,
		InitialBalance:     r.InitialBalance,
		InitialBalanceDate: ibd,
	}
}

type updateAccountReq struct {
	Name               *string                  `json:"name"`
	AccessMode         *model.AccountAccessMode `json:"accessMode"`
	Kind               *model.AccountKind       `json:"kind"`
	Icon               *string                  `json:"icon"`
	InitialBalance     *float64                 `json:"initialBalance"`
	InitialBalanceDate *string                  `json:"initialBalanceDate"` // "YYYY-MM-DD", nil = don't update
}

func (r *updateAccountReq) validate() error {
	if r.Name != nil {
		*r.Name = strings.TrimSpace(*r.Name)
		if *r.Name == "" {
			return fmt.Errorf("name cannot be empty")
		}
	}
	if r.AccessMode != nil && !r.AccessMode.IsValid() {
		return fmt.Errorf("accessMode must be 'personal' or 'shared'")
	}
	if r.Kind != nil {
		return fmt.Errorf("kind cannot be changed after account creation")
	}
	if r.InitialBalanceDate != nil {
		if _, err := time.Parse("2006-01-02", *r.InitialBalanceDate); err != nil {
			return fmt.Errorf("initialBalanceDate must be YYYY-MM-DD")
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
