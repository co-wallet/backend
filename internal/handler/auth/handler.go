package auth

import (
	"context"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service"
)

type authService interface {
	Login(ctx context.Context, email, password string) (model.User, service.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (service.TokenPair, error)
}

type userService interface {
	GetByID(ctx context.Context, id string) (model.User, error)
	ListActive(ctx context.Context) ([]model.User, error)
	UpdateCurrency(ctx context.Context, id, currency string) (model.User, error)
}

type Handler struct {
	auth  authService
	users userService
}

func New(auth authService, users userService) *Handler {
	return &Handler{auth: auth, users: users}
}

var (
	jsonResponse       = httputil.JSONResponse
	jsonError          = httputil.JSONError
	handleServiceError = httputil.HandleServiceError
)
