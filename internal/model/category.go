package model

import "time"

type CategoryType string

const (
	CategoryTypeExpense CategoryType = "expense"
	CategoryTypeIncome  CategoryType = "income"
)

func (t CategoryType) IsValid() bool {
	return t == CategoryTypeExpense || t == CategoryTypeIncome
}

type Category struct {
	ID        string
	UserID    string
	Name      string
	Type      CategoryType
	Icon      *string
	Hidden    bool
	CreatedAt time.Time
}

type CreateCategoryReq struct {
	Name string
	Type CategoryType
	Icon *string
}

type UpdateCategoryReq struct {
	Name *string
	Icon *string
}
