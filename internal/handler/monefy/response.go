package monefyhandler

import (
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/model"
	"time"
)

type accountResponse struct {
	SourceID           string            `json:"source_id"`
	Name               string            `json:"name"`
	Currency           string            `json:"currency"`
	Kind               model.AccountKind `json:"kind"`
	Icon               string            `json:"icon"`
	SourceIcon         int64             `json:"source_icon"`
	InitialBalance     string            `json:"initial_balance"`
	InitialBalanceDate time.Time         `json:"initial_balance_date"`
	FinalBalance       string            `json:"final_balance"`
	IncludedInTotal    bool              `json:"source_included_in_total"`
	DisabledAt         *time.Time        `json:"source_disabled_at"`
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
type previewResponse struct {
	ID                            string               `json:"preview_id"`
	SHA256                        string               `json:"sha256"`
	ExpiresAt                     time.Time            `json:"expires_at"`
	CanConfirm                    bool                 `json:"can_confirm"`
	RequiresExclusionConfirmation bool                 `json:"requires_exclusion_confirmation"`
	Counts                        map[string]int       `json:"counts"`
	PeriodFrom                    *time.Time           `json:"period_from"`
	PeriodTo                      *time.Time           `json:"period_to"`
	Currencies                    []string             `json:"currencies"`
	Accounts                      []accountResponse    `json:"accounts"`
	Categories                    []categoryResponse   `json:"categories"`
	Diagnostics                   []diagnosticResponse `json:"diagnostics"`
	Exclusions                    []exclusionResponse  `json:"exclusions"`
	Deleted                       map[string]int       `json:"deleted"`
}

func toPreview(p model.ImportPreview) previewResponse {
	r := previewResponse{ID: p.ID, SHA256: p.SHA256, ExpiresAt: p.ExpiresAt, CanConfirm: true, RequiresExclusionConfirmation: len(p.Report.Exclusions) > 0,
		Counts: map[string]int{"accounts": len(p.Accounts), "categories": len(p.Categories), "transactions": len(p.Report.Transactions), "transfers": len(p.Report.Transfers)}, PeriodFrom: p.PeriodFrom, PeriodTo: p.PeriodTo,
		Currencies: []string{}, Accounts: []accountResponse{}, Categories: []categoryResponse{}, Diagnostics: []diagnosticResponse{}, Exclusions: []exclusionResponse{}, Deleted: p.Report.Deleted}
	seen := map[string]bool{}
	for i, a := range p.Report.Accounts {
		options := p.Accounts[i]
		r.Accounts = append(r.Accounts, accountResponse{a.ID, a.Name, a.Currency, options.Kind, options.Icon, a.Icon, a.InitialBalance.String(), a.CreatedAt, options.Balance, a.IncludedInTotal, a.DisabledAt})
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
