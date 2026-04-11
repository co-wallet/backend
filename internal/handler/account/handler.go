package accounthandler

import (
	"context"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/model"
)

//go:generate mockgen -source=handler.go -destination=mocks/mock_account_service.go -package=mocks

type accountService interface {
	ListByUser(ctx context.Context, userID string) ([]model.Account, error)
	ListBalancesByUser(ctx context.Context, userID, displayCurrency string) (map[string]model.AccountBalance, error)
	GetByID(ctx context.Context, accountID string) (model.Account, error)
	CreateAccount(ctx context.Context, ownerID string, req model.CreateAccountReq) (model.Account, error)
	UpdateAccount(ctx context.Context, accountID string, req model.UpdateAccountReq) (model.Account, error)
	DeleteAccount(ctx context.Context, requesterID, accountID string) error
	AddMember(ctx context.Context, accountID, username string, share float64) ([]model.AccountMember, error)
	UpdateMember(ctx context.Context, accountID, memberUserID string, share float64) ([]model.AccountMember, error)
	RemoveMember(ctx context.Context, requesterID, accountID, memberUserID string) error
	GetMembers(ctx context.Context, accountID string) ([]model.AccountMember, error)
}

type userSource interface {
	GetByID(ctx context.Context, id string) (model.User, error)
}

type Handler struct {
	service accountService
	users   userSource
}

func New(svc accountService, users userSource) *Handler {
	return &Handler{service: svc, users: users}
}

var (
	jsonResponse       = httputil.JSONResponse
	jsonError          = httputil.JSONError
	handleServiceError = httputil.HandleServiceError
)
