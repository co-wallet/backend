package monefyhandler

import (
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/model"
	"sort"
	"time"
)

type importMemberResponse struct {
	UserID         string  `json:"user_id"`
	Username       string  `json:"username"`
	DefaultShare   float64 `json:"default_share"`
	InitialBalance string  `json:"initial_balance"`
	FinalBalance   string  `json:"final_balance"`
}
type accountResponse struct {
	AccessMode         model.AccountAccessMode `json:"access_mode"`
	Members            []importMemberResponse  `json:"members"`
	SourceID           string                  `json:"source_id"`
	Name               string                  `json:"name"`
	SourceName         string                  `json:"source_name"`
	Currency           string                  `json:"currency"`
	Kind               model.AccountKind       `json:"kind"`
	Icon               string                  `json:"icon"`
	SourceIcon         int64                   `json:"source_icon"`
	InitialBalance     string                  `json:"initial_balance"`
	InitialBalanceDate time.Time               `json:"initial_balance_date"`
	FinalBalance       string                  `json:"final_balance"`
	IncludedInTotal    bool                    `json:"source_included_in_total"`
	DisabledAt         *time.Time              `json:"source_disabled_at"`
}
type categoryResponse struct {
	SourceID   string     `json:"source_id"`
	ExistingID string     `json:"existing_id,omitempty"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Icon       string     `json:"icon"`
	SourceIcon int64      `json:"source_icon"`
	DisabledAt *time.Time `json:"source_disabled_at"`
}
type diagnosticResponse struct {
	Severity monefy.Severity `json:"severity"`
	Code     string          `json:"code"`
	Entity   string          `json:"entity"`
	SourceID string          `json:"source_id"`
	Message  string          `json:"message"`
}
type exclusionResponse struct {
	Entity   string `json:"entity"`
	SourceID string `json:"source_id"`
	Reason   string `json:"reason"`
}
type replacementAccountResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Currency  string     `json:"currency"`
	DeletedAt *time.Time `json:"deleted_at"`
}
type replacementResponse struct {
	Accounts []replacementAccountResponse `json:"accounts"`
	Counts   map[string]int               `json:"counts"`
	Blockers map[string]int               `json:"blockers"`
}
type previewResponse struct {
	CurrencyRates                 []currencyRateResponse `json:"currency_rates"`
	Mode                          model.ImportMode       `json:"mode"`
	Replacement                   *replacementResponse   `json:"replacement,omitempty"`
	ID                            string                 `json:"preview_id"`
	SHA256                        string                 `json:"sha256"`
	ExpiresAt                     time.Time              `json:"expires_at"`
	CanConfirm                    bool                   `json:"can_confirm"`
	RequiresExclusionConfirmation bool                   `json:"requires_exclusion_confirmation"`
	Counts                        map[string]int         `json:"counts"`
	PeriodFrom                    *time.Time             `json:"period_from"`
	PeriodTo                      *time.Time             `json:"period_to"`
	Currencies                    []string               `json:"currencies"`
	Accounts                      []accountResponse      `json:"accounts"`
	Categories                    []categoryResponse     `json:"categories"`
	Diagnostics                   []diagnosticResponse   `json:"diagnostics"`
	Exclusions                    []exclusionResponse    `json:"exclusions"`
	Deleted                       map[string]int         `json:"deleted"`
}

type currencyRateResponse struct {
	Currency     string `json:"currency"`
	BaseCurrency string `json:"base_currency"`
	Rate         string `json:"rate"`
	Source       string `json:"source"`
	Transactions int    `json:"transactions"`
}

func toPreview(p model.ImportPreview) previewResponse {
	r := previewResponse{ID: p.ID, SHA256: p.SHA256, ExpiresAt: p.ExpiresAt, CanConfirm: true, RequiresExclusionConfirmation: len(p.Report.Exclusions) > 0,
		Counts: map[string]int{"accounts": len(p.Accounts), "categories": len(p.Categories), "transactions": len(p.Report.Transactions), "transfers": len(p.Report.Transfers)}, PeriodFrom: p.PeriodFrom, PeriodTo: p.PeriodTo,
		Currencies: []string{}, Accounts: []accountResponse{}, Categories: []categoryResponse{}, Diagnostics: []diagnosticResponse{}, Exclusions: []exclusionResponse{}, Deleted: p.Report.Deleted}
	r.Mode = p.Mode
	r.CurrencyRates = []currencyRateResponse{}
	for _, rate := range p.CurrencyRates {
		r.CurrencyRates = append(r.CurrencyRates, currencyRateResponse{rate.Currency, rate.BaseCurrency, rate.Rate, rate.Source, rate.Transactions})
	}
	sort.Slice(r.CurrencyRates, func(i, j int) bool { return r.CurrencyRates[i].Currency < r.CurrencyRates[j].Currency })
	if r.Mode == "" {
		r.Mode = model.ImportEmpty
	}
	if p.Replacement != nil {
		r.Replacement = &replacementResponse{Accounts: []replacementAccountResponse{}, Counts: p.Replacement.Counts, Blockers: p.Replacement.Blockers}
		for _, a := range p.Replacement.Accounts {
			r.Replacement.Accounts = append(r.Replacement.Accounts, replacementAccountResponse{a.ID, a.Name, a.Currency, a.DeletedAt})
		}
	}
	seen := map[string]bool{}
	for i, a := range p.Report.Accounts {
		options := p.Accounts[i]
		members := make([]importMemberResponse, 0, len(options.Members))
		for _, m := range options.Members {
			members = append(members, importMemberResponse{m.UserID, m.Username, m.DefaultShare, m.InitialBalance, m.FinalBalance})
		}
		r.Accounts = append(r.Accounts, accountResponse{options.AccessMode, members, a.ID, options.Name, a.Name, a.Currency, options.Kind, options.Icon, a.Icon, a.InitialBalance.String(), a.CreatedAt, options.Balance, a.IncludedInTotal, a.DisabledAt})
		if !seen[a.Currency] {
			r.Currencies = append(r.Currencies, a.Currency)
			seen[a.Currency] = true
		}
	}
	for i, c := range p.Categories {
		source := p.Report.Categories[i]
		r.Categories = append(r.Categories, categoryResponse{c.SourceID, c.ExistingID, c.Name, c.Type, c.Icon, source.Icon, source.DisabledAt})
	}
	for _, d := range p.Report.Diagnostics {
		r.Diagnostics = append(r.Diagnostics, diagnosticResponse{d.Severity, d.Code, d.Entity, d.SourceID, d.Message})
		if d.Severity == monefy.Blocking {
			r.CanConfirm = false
		}
	}
	for _, e := range p.Report.Exclusions {
		r.Exclusions = append(r.Exclusions, exclusionResponse{e.Entity, e.ID, e.Reason})
	}
	return r
}

type resultResponse struct {
	PreviewID        string    `json:"preview_id"`
	Accounts         int       `json:"accounts"`
	Categories       int       `json:"categories"`
	ReusedCategories int       `json:"reused_categories"`
	Transactions     int       `json:"transactions"`
	Transfers        int       `json:"transfers"`
	CompletedAt      time.Time `json:"completed_at"`
}

func toResult(r model.ImportResult) resultResponse {
	return resultResponse{r.PreviewID, r.Accounts, r.Categories, r.ReusedCategories, r.Transactions, r.Transfers, r.CompletedAt}
}
