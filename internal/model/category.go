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
	ParentID  *string
	Name      string
	Type      CategoryType
	Icon      *string
	CreatedAt time.Time
}

type CreateCategoryReq struct {
	ParentID *string
	Name     string
	Type     CategoryType
	Icon     *string
}

type UpdateCategoryReq struct {
	Name *string
	Icon *string
}
