package analytics

import (
	"github.com/co-wallet/backend/internal/model"
)

type SummaryResponse struct {
	Balance  float64 `json:"balance"`
	Expenses float64 `json:"expenses"`
	Income   float64 `json:"income"`
}

func toSummaryResponse(s model.AnalyticsSummary) SummaryResponse {
	return SummaryResponse{
		Balance:  s.Balance,
		Expenses: s.Expenses,
		Income:   s.Income,
	}
}

type CategoryStatResponse struct {
	CategoryID   string  `json:"categoryId"`
	CategoryName string  `json:"categoryName"`
	Icon         *string `json:"icon,omitempty"`
	Amount       float64 `json:"amount"`
}

func toCategoryStatResponses(stats []model.CategoryStat) []CategoryStatResponse {
	out := make([]CategoryStatResponse, len(stats))
	for i, s := range stats {
		out[i] = CategoryStatResponse{
			CategoryID:   s.CategoryID,
			CategoryName: s.CategoryName,
			Icon:         s.Icon,
			Amount:       s.Amount,
		}
	}
	return out
}

type TagStatResponse struct {
	TagID   string  `json:"tagId"`
	TagName string  `json:"tagName"`
	Amount  float64 `json:"amount"`
}

func toTagStatResponses(stats []model.TagStat) []TagStatResponse {
	out := make([]TagStatResponse, len(stats))
	for i, s := range stats {
		out[i] = TagStatResponse{
			TagID:   s.TagID,
			TagName: s.TagName,
			Amount:  s.Amount,
		}
	}
	return out
}
