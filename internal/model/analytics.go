package model

import "time"

type AmountMode string

const (
	AmountModeShare AmountMode = "share"
	AmountModeTotal AmountMode = "total"
)

func (m AmountMode) IsValid() bool {
	return m == AmountModeShare || m == AmountModeTotal
}

type AnalyticsSummary struct {
	ExpensesMissingAmounts int
	IncomeMissingAmounts   int
	Balance                float64
	Expenses               float64
	Income                 float64
}

type CategoryStat struct {
	CategoryID     string
	CategoryName   string
	Icon           *string
	Amount         float64
	MissingAmounts int
}

type TagStat struct {
	TagID          string
	TagName        string
	Amount         float64
	MissingAmounts int
}

type AnalyticsFilter struct {
	AmountMode              AmountMode
	IncludeTransferExpenses bool
	IncludeTransferIncome   bool
	UserID                  string
	DateFrom                time.Time
	DateTo                  time.Time
	AccountIDs              []string
	AccountKinds            []AccountKind
	CategoryIDs             []string
	TagIDs                  []string
	TagMode                 string
	WithoutTags             bool
	WithoutCategory         bool
	DisplayCurrency         string          // convert all amounts to this currency (default: USD)
	TxType                  TransactionType // "expense" (default) or "income"
}
