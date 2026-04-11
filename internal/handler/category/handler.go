package categoryhandler

import (
	"context"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

//go:generate mockgen -source=handler.go -destination=mocks/mock_category_service.go -package=mocks

type categoryService interface {
	Create(ctx context.Context, userID string, req model.CreateCategoryReq) (model.Category, error)
	List(ctx context.Context, userID string, catType model.CategoryType) ([]service.CategoryNode, error)
	Update(ctx context.Context, userID, id string, req model.UpdateCategoryReq) (model.Category, error)
	Delete(ctx context.Context, userID, id string) error
}

type Handler struct {
	service categoryService
}

func New(svc categoryService) *Handler {
	return &Handler{service: svc}
}

var (
	jsonResponse       = httputil.JSONResponse
	jsonError          = httputil.JSONError
	handleServiceError = httputil.HandleServiceError
)
