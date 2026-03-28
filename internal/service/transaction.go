package service

import (
	"context"
	"fmt"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

//go:generate mockgen -destination=mocks/mock_transaction_repo.go -package=mocks github.com/co-wallet/backend/internal/service TransactionRepo
type TransactionRepo interface {
	Create(ctx context.Context, tx model.Transaction) (model.Transaction, error)
	GetByID(ctx context.Context, id string) (model.Transaction, error)
	List(ctx context.Context, userID string, f model.TransactionFilter) ([]model.Transaction, error)
	Update(ctx context.Context, tx model.Transaction) (model.Transaction, error)
	Delete(ctx context.Context, id string) error
	GetMemberDefaults(ctx context.Context, accountID string) ([]model.AccountMember, error)
}

//go:generate mockgen -destination=mocks/mock_account_repo_tx.go -package=mocks github.com/co-wallet/backend/internal/service AccountRepoForTx
type AccountRepoForTx interface {
	GetByID(ctx context.Context, id string) (model.Account, error)
	IsMember(ctx context.Context, accountID, userID string) (bool, error)
}

type TransactionService struct {
	repo    TransactionRepo
	accounts AccountRepoForTx
}

func NewTransactionService(repo *repository.TransactionRepository, accounts *repository.AccountRepository) *TransactionService {
	return &TransactionService{repo: repo, accounts: accounts}
}

func (s *TransactionService) Create(ctx context.Context, userID string, req model.CreateTransactionReq) (model.Transaction, error) {
	if err := s.validateCreate(req); err != nil {
		return model.Transaction{}, err
	}

	// Verify the user is a member of the account
	isMember, err := s.accounts.IsMember(ctx, req.AccountID, userID)
	if err != nil {
		return model.Transaction{}, err
	}
	if !isMember {
		return model.Transaction{}, fmt.Errorf("not a member of account: %w", apperr.ErrForbidden)
	}

	tx := model.Transaction{
		AccountID:        req.AccountID,
		ToAccountID:      req.ToAccountID,
		Type:             req.Type,
		Amount:           req.Amount,
		Currency:         req.Currency,
		ExchangeRate:     req.ExchangeRate,
		CategoryID:       req.CategoryID,
		Description:      req.Description,
		Date:             req.Date,
		IncludeInBalance: req.IncludeInBalance,
		CreatedBy:        userID,
	}

	// Calculate shares for shared accounts
	tx.Shares, err = s.resolveShares(ctx, req)
	if err != nil {
		return model.Transaction{}, err
	}

	return s.repo.Create(ctx, tx)
}

func (s *TransactionService) GetByID(ctx context.Context, userID, id string) (model.Transaction, error) {
	tx, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return model.Transaction{}, err
	}
	isMember, err := s.accounts.IsMember(ctx, tx.AccountID, userID)
	if err != nil {
		return model.Transaction{}, err
	}
	if !isMember {
		return model.Transaction{}, fmt.Errorf("not a member of account: %w", apperr.ErrForbidden)
	}
	return tx, nil
}

func (s *TransactionService) List(ctx context.Context, userID string, f model.TransactionFilter) ([]model.Transaction, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	return s.repo.List(ctx, userID, f)
}

func (s *TransactionService) Update(ctx context.Context, userID, id string, req model.UpdateTransactionReq) (model.Transaction, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return model.Transaction{}, err
	}
	isMember, err := s.accounts.IsMember(ctx, existing.AccountID, userID)
	if err != nil {
		return model.Transaction{}, err
	}
	if !isMember {
		return model.Transaction{}, fmt.Errorf("not a member of account: %w", apperr.ErrForbidden)
	}

	if req.Amount != nil {
		if *req.Amount <= 0 {
			return model.Transaction{}, fmt.Errorf("amount must be positive: %w", apperr.ErrValidation)
		}
		existing.Amount = *req.Amount
	}
	if req.CategoryID != nil {
		existing.CategoryID = req.CategoryID
	}
	if req.Description != nil {
		existing.Description = req.Description
	}
	if req.Date != nil {
		existing.Date = *req.Date
	}
	if req.IncludeInBalance != nil {
		existing.IncludeInBalance = *req.IncludeInBalance
	}

	if req.Shares != nil {
		if err := validateCustomShares(existing.Amount, req.Shares); err != nil {
			return model.Transaction{}, fmt.Errorf("%w: %w", apperr.ErrValidation, err)
		}
		existing.Shares = make([]model.TransactionShare, len(req.Shares))
		for i, s := range req.Shares {
			existing.Shares[i] = model.TransactionShare{UserID: s.UserID, Amount: s.Amount, IsCustom: true}
		}
	}

	return s.repo.Update(ctx, existing)
}

func (s *TransactionService) Delete(ctx context.Context, userID, id string) error {
	tx, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	isMember, err := s.accounts.IsMember(ctx, tx.AccountID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return fmt.Errorf("not a member of account: %w", apperr.ErrForbidden)
	}
	return s.repo.Delete(ctx, id)
}

// --- internal ---

func (s *TransactionService) validateCreate(req model.CreateTransactionReq) error {
	if !req.Type.IsValid() {
		return fmt.Errorf("invalid transaction type: %w", apperr.ErrValidation)
	}
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be positive: %w", apperr.ErrValidation)
	}
	if len(req.Currency) != 3 {
		return fmt.Errorf("currency must be a 3-letter ISO code: %w", apperr.ErrValidation)
	}
	if req.Date.IsZero() {
		return fmt.Errorf("date is required: %w", apperr.ErrValidation)
	}
	if req.Type == model.TransactionTypeTransfer && req.ToAccountID == nil {
		return fmt.Errorf("to_account_id is required for transfer: %w", apperr.ErrValidation)
	}
	return nil
}

func (s *TransactionService) resolveShares(ctx context.Context, req model.CreateTransactionReq) ([]model.TransactionShare, error) {
	if req.Shares != nil {
		// Custom shares provided — validate and use them
		if err := validateCustomShares(req.Amount, req.Shares); err != nil {
			return nil, fmt.Errorf("%w: %w", apperr.ErrValidation, err)
		}
		shares := make([]model.TransactionShare, len(req.Shares))
		for i, s := range req.Shares {
			shares[i] = model.TransactionShare{UserID: s.UserID, Amount: s.Amount, IsCustom: true}
		}
		return shares, nil
	}

	// Auto-calculate from account member defaults
	members, err := s.repo.GetMemberDefaults(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}
	if len(members) <= 1 {
		// Personal account or single member — no split needed
		return nil, nil
	}
	return calculateShares(req.Amount, members)
}
