package model

import "time"

type CategoryType string

const (
	CategoryTypeExpense CategoryType = "expense"
	CategoryTypeIncome  CategoryType = "income"
)

type Category struct {
	ID        string       `json:"id"`
	UserID    string       `json:"userId"`
	ParentID  *string      `json:"parentId"`
	Name      string       `json:"name"`
	Type      CategoryType `json:"type"`
	Icon      *string      `json:"icon"`
	CreatedAt time.Time    `json:"createdAt"`
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
