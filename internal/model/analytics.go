package model

import "time"

type AnalyticsSummary struct {
	Balance  float64
	Expenses float64
	Income   float64
}

type CategoryStat struct {
	CategoryID   string
	CategoryName string
	Icon         *string
	Amount       float64
}

type TagStat struct {
	TagID   string
	TagName string
	Amount  float64
}

type AnalyticsFilter struct {
	UserID          string
	DateFrom        time.Time
	DateTo          time.Time
	AccountIDs      []string
	DisplayCurrency string          // convert all amounts to this currency (default: USD)
	TxType          TransactionType // "expense" (default) or "income"
}
