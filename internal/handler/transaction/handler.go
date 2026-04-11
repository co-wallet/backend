package transactionhandler

import (
	"context"

	"github.com/co-wallet/backend/internal/httputil"
	"github.com/co-wallet/backend/internal/model"
)

//go:generate mockgen -source=handler.go -destination=mocks/mock_transaction_service.go -package=mocks

type transactionService interface {
	Create(ctx context.Context, userID string, req model.CreateTransactionReq) (model.Transaction, error)
	GetByID(ctx context.Context, userID, id string) (model.Transaction, error)
	List(ctx context.Context, userID string, f model.TransactionFilter) ([]model.Transaction, error)
	Update(ctx context.Context, userID, id string, req model.UpdateTransactionReq) (model.Transaction, error)
	Delete(ctx context.Context, userID, id string) error
}

type Handler struct {
	service transactionService
}

func New(svc transactionService) *Handler {
	return &Handler{service: svc}
}

var (
	jsonResponse       = httputil.JSONResponse
	jsonError          = httputil.JSONError
	handleServiceError = httputil.HandleServiceError
)
