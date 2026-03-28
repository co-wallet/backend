package categoryhandler

import (
	"time"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type CategoryResponse struct {
	ID        string             `json:"id"`
	UserID    string             `json:"userId"`
	ParentID  *string            `json:"parentId"`
	Name      string             `json:"name"`
	Type      model.CategoryType `json:"type"`
	Icon      *string            `json:"icon"`
	CreatedAt time.Time          `json:"createdAt"`
}

type CategoryNodeResponse struct {
	CategoryResponse
	Children []CategoryNodeResponse `json:"children"`
}

func toCategoryResponse(c model.Category) CategoryResponse {
	return CategoryResponse{
		ID:        c.ID,
		UserID:    c.UserID,
		ParentID:  c.ParentID,
		Name:      c.Name,
		Type:      c.Type,
		Icon:      c.Icon,
		CreatedAt: c.CreatedAt,
	}
}

func toCategoryNodeResponse(node service.CategoryNode) CategoryNodeResponse {
	resp := CategoryNodeResponse{
		CategoryResponse: toCategoryResponse(node.Category),
	}
	if len(node.Children) > 0 {
		resp.Children = make([]CategoryNodeResponse, len(node.Children))
		for i, child := range node.Children {
			resp.Children[i] = toCategoryNodeResponse(child)
		}
	}
	return resp
}

func toCategoryNodeResponses(nodes []service.CategoryNode) []CategoryNodeResponse {
	resp := make([]CategoryNodeResponse, len(nodes))
	for i, n := range nodes {
		resp[i] = toCategoryNodeResponse(n)
	}
	return resp
}
