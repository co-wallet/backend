package taghandler

import (
	"context"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/model"
)

//go:generate mockgen -source=handler.go -destination=mocks/mock_tag_service.go -package=mocks

type tagService interface {
	List(ctx context.Context, userID, q string) ([]model.TagWithCount, error)
	Rename(ctx context.Context, userID, id, name string) (model.Tag, error)
	Delete(ctx context.Context, userID, id string) error
}

type Handler struct {
	service tagService
}

func New(svc tagService) *Handler {
	return &Handler{service: svc}
}

var (
	jsonResponse       = httputil.JSONResponse
	jsonError          = httputil.JSONError
	handleServiceError = httputil.HandleServiceError
)
