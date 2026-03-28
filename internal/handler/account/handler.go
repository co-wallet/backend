package accounthandler

import (
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
)

type Handler struct {
	service  *service.AccountService
	accounts *repository.AccountRepository
}

func New(svc *service.AccountService, accounts *repository.AccountRepository) *Handler {
	return &Handler{service: svc, accounts: accounts}
}
