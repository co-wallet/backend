package model

type AnalyticsSummary struct {
	Balance  float64 `json:"balance"`
	Expenses float64 `json:"expenses"`
	Income   float64 `json:"income"`
}

type CategoryStat struct {
	CategoryID   string  `json:"categoryId"`
	CategoryName string  `json:"categoryName"`
	Icon         *string `json:"icon,omitempty"`
	Amount       float64 `json:"amount"`
}

type TagStat struct {
	TagID   string  `json:"tagId"`
	TagName string  `json:"tagName"`
	Amount  float64 `json:"amount"`
}

type AnalyticsFilter struct {
	UserID          string
	DateFrom        string
	DateTo          string
	AccountIDs      []string
	DisplayCurrency string // convert all amounts to this currency (default: USD)
}
