package taghandler

import (
	"github.com/co-wallet/backend/internal/model"
)

type tagResponse struct {
	Hidden  bool   `json:"hidden"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	TxCount int    `json:"txCount,omitempty"`
}

func toTagResponse(t model.Tag) tagResponse {
	return tagResponse{ID: t.ID, Name: t.Name, Hidden: t.Hidden}
}

func toTagWithCountResponses(tags []model.TagWithCount) []tagResponse {
	out := make([]tagResponse, len(tags))
	for i, t := range tags {
		out[i] = tagResponse{ID: t.ID, Name: t.Name, TxCount: t.TxCount, Hidden: t.Hidden}
	}
	return out
}
