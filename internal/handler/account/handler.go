package accounthandler

import (
	"context"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type userSource interface {
	GetByID(ctx context.Context, id string) (*model.User, error)
}

type Handler struct {
	service *service.AccountService
	users   userSource
}

func New(svc *service.AccountService, users userSource) *Handler {
	return &Handler{service: svc, users: users}
}

var (
	jsonResponse       = httputil.JSONResponse
	jsonError          = httputil.JSONError
	handleServiceError = httputil.HandleServiceError
)
