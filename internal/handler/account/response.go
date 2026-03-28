package accounthandler

import "github.com/co-wallet/backend/internal/model"

type BalanceResponse struct {
	Native          float64 `json:"native"`
	Display         float64 `json:"display"`
	TotalNative     float64 `json:"totalNative"`
	TotalDisplay    float64 `json:"totalDisplay"`
	DisplayCurrency string  `json:"displayCurrency"`
}

type AccountResponse struct {
	ID                 string           `json:"id"`
	OwnerID            string           `json:"ownerId"`
	Name               string           `json:"name"`
	Type               string           `json:"type"`
	Currency           string           `json:"currency"`
	Icon               *string          `json:"icon"`
	IncludeInBalance   bool             `json:"includeInBalance"`
	InitialBalance     float64          `json:"initialBalance"`
	InitialBalanceDate *string          `json:"initialBalanceDate"`
	Members            []MemberResponse `json:"members,omitempty"`
	Balance            *BalanceResponse `json:"balance,omitempty"`
}

type MemberResponse struct {
	AccountID    string  `json:"accountId"`
	UserID       string  `json:"userId"`
	Username     string  `json:"username"`
	DefaultShare float64 `json:"defaultShare"`
}

func toAccountResponse(a model.Account) AccountResponse {
	return AccountResponse{
		ID:                 a.ID,
		OwnerID:            a.OwnerID,
		Name:               a.Name,
		Type:               string(a.Type),
		Currency:           a.Currency,
		Icon:               a.Icon,
		IncludeInBalance:   a.IncludeInBalance,
		InitialBalance:     a.InitialBalance,
		InitialBalanceDate: a.InitialBalanceDate,
	}
}

func toAccountResponseWithMembers(a model.Account, members []model.AccountMember) AccountResponse {
	resp := toAccountResponse(a)
	resp.Members = toMemberResponses(members)
	return resp
}

func toMemberResponse(m model.AccountMember) MemberResponse {
	return MemberResponse{
		AccountID:    m.AccountID,
		UserID:       m.UserID,
		Username:     m.Username,
		DefaultShare: m.DefaultShare,
	}
}

func toMemberResponses(members []model.AccountMember) []MemberResponse {
	out := make([]MemberResponse, len(members))
	for i, m := range members {
		out[i] = toMemberResponse(m)
	}
	return out
}
