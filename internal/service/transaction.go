package service

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/db"
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
	GetTransferDestination(ctx context.Context, id string) (model.Account, error)
	GetByID(ctx context.Context, id string) (model.Account, error)
	IsMember(ctx context.Context, accountID, userID string) (bool, error)
}

type TransactionService struct {
	withTx   func(context.Context, func(*TransactionService) error) error
	repo     TransactionRepo
	accounts AccountRepoForTx
	tags     TagRepo
}

func NewTransactionService(pool *pgxpool.Pool, repo *repository.TransactionRepository, accounts *repository.AccountRepository, tags *repository.TagRepository) *TransactionService {
	return &TransactionService{repo: repo, accounts: accounts, tags: tags, withTx: func(ctx context.Context, fn func(*TransactionService) error) error {
		return db.WithTx(ctx, pool, func(tx pgx.Tx) error {
			return fn(&TransactionService{repo: repo.WithTx(tx), accounts: accounts.WithTx(tx), tags: tags.WithTx(tx)})
		})
	}}
}

func (s *TransactionService) Create(ctx context.Context, userID string, req model.CreateTransactionReq) (model.Transaction, error) {
	if s.withTx != nil {
		var result model.Transaction
		err := s.withTx(ctx, func(scoped *TransactionService) error {
			var err error
			result, err = scoped.Create(ctx, userID, req)
			return err
		})
		return result, err
	}

	if err := s.validateCreate(req); err != nil {
		return model.Transaction{}, err
	}

	isMember, err := s.accounts.IsMember(ctx, req.AccountID, userID)
	if err != nil {
		return model.Transaction{}, err
	}
	if !isMember {
		return model.Transaction{}, fmt.Errorf("not a member of account: %w", apperr.ErrForbidden)
	}

	tx := model.Transaction{
		AccountID:             req.AccountID,
		ToAccountID:           req.ToAccountID,
		ToAmount:              req.ToAmount,
		Type:                  req.Type,
		Amount:                req.Amount,
		Currency:              req.Currency,
		ExchangeRate:          req.ExchangeRate,
		DefaultCurrency:       req.DefaultCurrency,
		DefaultCurrencyAmount: req.DefaultCurrencyAmount,
		CategoryID:            req.CategoryID,
		Description:           req.Description,
		Date:                  req.Date,
		CreatedBy:             userID,
	}

	if tx.Type == model.TransactionTypeTransfer {
		source, err := s.accounts.GetByID(ctx, tx.AccountID)
		if err != nil {
			return model.Transaction{}, err
		}
		destination, err := s.accounts.GetTransferDestination(ctx, *tx.ToAccountID)
		if err != nil {
			return model.Transaction{}, err
		}
		member, err := s.accounts.IsMember(ctx, destination.ID, userID)
		if err != nil {
			return model.Transaction{}, err
		}
		if !member && (source.AccessMode == model.AccountAccessModeShared || destination.AccessMode == model.AccountAccessModeShared) {
			return model.Transaction{}, fmt.Errorf("external transfers require personal source and destination accounts: %w", apperr.ErrForbidden)
		}
		if !member && !destination.AcceptTransfers {
			return model.Transaction{}, fmt.Errorf("destination unavailable: %w", apperr.ErrForbidden)
		}
		if tx.Currency != source.Currency {
			return model.Transaction{}, fmt.Errorf("source currency mismatch: %w", apperr.ErrValidation)
		}
		tx.Account, tx.AccountTo = source, &destination
		if err := validateTransferAmount(tx); err != nil {
			return model.Transaction{}, err
		}
	}

	tx.Shares, err = s.resolveShares(ctx, req, userID)
	if err != nil {
		return model.Transaction{}, err
	}

	tx, err = s.repo.Create(ctx, tx)
	if err != nil {
		return model.Transaction{}, err
	}

	if len(req.Tags) > 0 {
		tx.Tags, err = s.tags.UpsertForTransaction(ctx, tx.ID, userID, req.Tags)
		if err != nil {
			return model.Transaction{}, err
		}
	}
	return tx, nil
}

func (s *TransactionService) GetByID(ctx context.Context, userID, id string) (model.Transaction, error) {
	tx, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return model.Transaction{}, err
	}
	if err = s.checkTransactionAccess(ctx, tx, userID); err != nil {
		return model.Transaction{}, err
	}
	tx.Tags, err = s.tags.ListForTransaction(ctx, id)
	if err != nil {
		return model.Transaction{}, err
	}
	if tx.Type == model.TransactionTypeTransfer {
		member, err := s.accounts.IsMember(ctx, tx.AccountID, userID)
		if err != nil {
			return model.Transaction{}, err
		}
		tx.ReadOnly = !member
	}
	return transferView(tx), nil
}

func (s *TransactionService) List(ctx context.Context, userID string, f model.TransactionFilter) ([]model.Transaction, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	txs, err := s.repo.List(ctx, userID, f)
	if err != nil {
		return nil, err
	}
	if len(txs) == 0 {
		return txs, nil
	}
	ids := make([]string, len(txs))
	for i := range txs {
		ids[i] = txs[i].ID
	}
	tagsByTx, err := s.tags.ListForTransactions(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range txs {
		txs[i].Tags = tagsByTx[txs[i].ID]
		txs[i] = transferView(txs[i])
	}
	return txs, nil
}

func (s *TransactionService) Update(ctx context.Context, userID, id string, req model.UpdateTransactionReq) (model.Transaction, error) {
	if s.withTx != nil {
		var result model.Transaction
		err := s.withTx(ctx, func(scoped *TransactionService) error {
			var err error
			result, err = scoped.Update(ctx, userID, id, req)
			return err
		})
		return result, err
	}

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
		if *req.Amount <= 0 || math.IsNaN(*req.Amount) || math.IsInf(*req.Amount, 0) {
			return model.Transaction{}, fmt.Errorf("amount must be positive: %w", apperr.ErrValidation)
		}
		existing.Amount = *req.Amount
	}
	if req.ToAmount != nil {
		existing.ToAmount = req.ToAmount
	}
	if existing.Type == model.TransactionTypeTransfer {
		if req.Amount != nil && req.ToAmount == nil && existing.AccountTo != nil && existing.AccountTo.Currency == existing.Currency {
			existing.ToAmount = req.Amount
		}
		if err := validateTransferAmount(existing); err != nil {
			return model.Transaction{}, err
		}
	} else if req.ToAmount != nil {
		return model.Transaction{}, fmt.Errorf("to_amount requires a transfer: %w", apperr.ErrValidation)
	}
	if req.DefaultCurrency != nil {
		existing.DefaultCurrency = req.DefaultCurrency
	}
	if req.DefaultCurrencyAmount != nil {
		existing.DefaultCurrencyAmount = req.DefaultCurrencyAmount
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

	if req.Shares != nil {
		if err := validateCustomShares(existing.Amount, req.Shares); err != nil {
			return model.Transaction{}, fmt.Errorf("%w: %w", apperr.ErrValidation, err)
		}
		existing.Shares = make([]model.TransactionShare, len(req.Shares))
		for i, sh := range req.Shares {
			existing.Shares[i] = model.TransactionShare{UserID: sh.UserID, Amount: sh.Amount, IsCustom: true}
		}
	} else if req.Amount != nil {
		// Amount changed but no explicit shares provided — recalculate shares
		// to keep transaction_shares in sync with the new amount.
		existing.Shares, err = s.recalcShares(ctx, existing)
		if err != nil {
			return model.Transaction{}, err
		}
	}

	tx, err := s.repo.Update(ctx, existing)
	if err != nil {
		return model.Transaction{}, err
	}

	if req.Tags != nil {
		tx.Tags, err = s.tags.UpsertForTransaction(ctx, id, userID, req.Tags)
		if err != nil {
			return model.Transaction{}, err
		}
	} else {
		tx.Tags, err = s.tags.ListForTransaction(ctx, id)
		if err != nil {
			return model.Transaction{}, err
		}
	}
	return tx, nil
}

func (s *TransactionService) Delete(ctx context.Context, userID, id string) error {
	tx, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	member, err := s.accounts.IsMember(ctx, tx.AccountID, userID)
	if err != nil {
		return err
	}
	if !member {
		return fmt.Errorf("only source members can delete: %w", apperr.ErrForbidden)
	}
	return s.repo.Delete(ctx, id)
}

// checkTransactionAccess verifies the user is a member of the source or destination account.
func (s *TransactionService) checkTransactionAccess(ctx context.Context, tx model.Transaction, userID string) error {
	isMember, err := s.accounts.IsMember(ctx, tx.AccountID, userID)
	if err != nil {
		return err
	}
	if !isMember && tx.ToAccountID != nil {
		isMember, err = s.accounts.IsMember(ctx, *tx.ToAccountID, userID)
		if err != nil {
			return err
		}
	}
	if !isMember {
		return fmt.Errorf("not a member of account: %w", apperr.ErrForbidden)
	}
	return nil
}

// --- internal ---

func (s *TransactionService) validateCreate(req model.CreateTransactionReq) error {
	if !req.Type.IsValid() {
		return fmt.Errorf("invalid transaction type: %w", apperr.ErrValidation)
	}
	if req.Amount <= 0 || math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) {
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
	if req.Type != model.TransactionTypeTransfer && (req.ToAccountID != nil || req.ToAmount != nil) {
		return fmt.Errorf("destination requires a transfer: %w", apperr.ErrValidation)
	}
	if req.ToAccountID != nil && *req.ToAccountID == req.AccountID {
		return fmt.Errorf("accounts must differ: %w", apperr.ErrValidation)
	}
	return nil
}

func (s *TransactionService) recalcShares(ctx context.Context, tx model.Transaction) ([]model.TransactionShare, error) {
	hasCustom := false
	for _, sh := range tx.Shares {
		if sh.IsCustom {
			hasCustom = true
			break
		}
	}
	if hasCustom {
		// Custom shares: scale proportionally to the new amount.
		oldTotal := 0.0
		for _, sh := range tx.Shares {
			oldTotal += sh.Amount
		}
		if oldTotal == 0 {
			oldTotal = 1
		}
		shares := make([]model.TransactionShare, len(tx.Shares))
		var distributed float64
		for i, sh := range tx.Shares {
			shares[i] = model.TransactionShare{UserID: sh.UserID, IsCustom: true}
			if i < len(tx.Shares)-1 {
				shares[i].Amount = math.Round(sh.Amount/oldTotal*tx.Amount*100) / 100
				distributed += shares[i].Amount
			} else {
				shares[i].Amount = math.Round((tx.Amount-distributed)*100) / 100
			}
		}
		return shares, nil
	}

	// Default shares: recalculate from member defaults.
	members, err := s.repo.GetMemberDefaults(ctx, tx.AccountID)
	if err != nil {
		return nil, err
	}
	if len(members) <= 1 {
		return []model.TransactionShare{{UserID: tx.CreatedBy, Amount: tx.Amount}}, nil
	}
	return calculateShares(tx.Amount, members)
}

func (s *TransactionService) resolveShares(ctx context.Context, req model.CreateTransactionReq, createdBy string) ([]model.TransactionShare, error) {
	if req.Shares != nil {
		if err := validateCustomShares(req.Amount, req.Shares); err != nil {
			return nil, fmt.Errorf("%w: %w", apperr.ErrValidation, err)
		}
		shares := make([]model.TransactionShare, len(req.Shares))
		for i, s := range req.Shares {
			shares[i] = model.TransactionShare{UserID: s.UserID, Amount: s.Amount, IsCustom: true}
		}
		return shares, nil
	}

	members, err := s.repo.GetMemberDefaults(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}
	// Personal account or shared with only one member: full amount goes to the creator.
	if len(members) <= 1 {
		return []model.TransactionShare{{UserID: createdBy, Amount: req.Amount}}, nil
	}
	return calculateShares(req.Amount, members)
}
