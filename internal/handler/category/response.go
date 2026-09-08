package categoryhandler

import (
	"time"

	"github.com/co-wallet/backend/internal/model"
)

type CategoryResponse struct {
	ID        string             `json:"id"`
	UserID    string             `json:"userId"`
	Name      string             `json:"name"`
	Type      model.CategoryType `json:"type"`
	Icon      *string            `json:"icon"`
	CreatedAt time.Time          `json:"createdAt"`
}

func toCategoryResponse(c model.Category) CategoryResponse {
	return CategoryResponse{
		ID:        c.ID,
		UserID:    c.UserID,
		Name:      c.Name,
		Type:      c.Type,
		Icon:      c.Icon,
		CreatedAt: c.CreatedAt,
	}
}

func toCategoryResponses(categories []model.Category) []CategoryResponse {
	resp := make([]CategoryResponse, len(categories))
	for i, category := range categories {
		resp[i] = toCategoryResponse(category)
	}
	return resp
}
