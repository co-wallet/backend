package analytics

import (
	"github.com/co-wallet/backend/internal/model"
)

type SummaryResponse struct {
	ExpensesMissingAmounts int     `json:"expensesMissingAmounts"`
	IncomeMissingAmounts   int     `json:"incomeMissingAmounts"`
	Balance                float64 `json:"balance"`
	Expenses               float64 `json:"expenses"`
	Income                 float64 `json:"income"`
}

func toSummaryResponse(s model.AnalyticsSummary) SummaryResponse {
	return SummaryResponse{
		ExpensesMissingAmounts: s.ExpensesMissingAmounts,
		IncomeMissingAmounts:   s.IncomeMissingAmounts,
		Balance:                s.Balance,
		Expenses:               s.Expenses,
		Income:                 s.Income,
	}
}

type CategoryStatResponse struct {
	CategoryID     string  `json:"categoryId"`
	CategoryName   string  `json:"categoryName"`
	Icon           *string `json:"icon,omitempty"`
	MissingAmounts int     `json:"missingAmounts"`
	Amount         float64 `json:"amount"`
}

func toCategoryStatResponses(stats []model.CategoryStat) []CategoryStatResponse {
	out := make([]CategoryStatResponse, len(stats))
	for i, s := range stats {
		out[i] = CategoryStatResponse{
			CategoryID:     s.CategoryID,
			CategoryName:   s.CategoryName,
			Icon:           s.Icon,
			Amount:         s.Amount,
			MissingAmounts: s.MissingAmounts,
		}
	}
	return out
}

type TagStatResponse struct {
	TagID          string  `json:"tagId"`
	TagName        string  `json:"tagName"`
	MissingAmounts int     `json:"missingAmounts"`
	Amount         float64 `json:"amount"`
}

func toTagStatResponses(stats []model.TagStat) []TagStatResponse {
	out := make([]TagStatResponse, len(stats))
	for i, s := range stats {
		out[i] = TagStatResponse{
			TagID:          s.TagID,
			TagName:        s.TagName,
			Amount:         s.Amount,
			MissingAmounts: s.MissingAmounts,
		}
	}
	return out
}
