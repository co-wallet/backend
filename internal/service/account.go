package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/db"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

//go:generate mockgen -source=account.go -destination=mocks/mock_account_repo.go -package=mocks

type accountRepo interface {
	ListTransferAccounts(ctx context.Context, username string) ([]model.TransferAccount, error)
	ListByUser(ctx context.Context, userID string) ([]model.Account, error)
	ListBalancesByUser(ctx context.Context, userID, displayCurrency string) (map[string]model.AccountBalance, error)
	GetByID(ctx context.Context, id string) (model.Account, error)
	Create(ctx context.Context, a model.Account) (model.Account, error)
	Update(ctx context.Context, a model.Account) (model.Account, error)
	SoftDelete(ctx context.Context, id string) error
	HasTransactions(ctx context.Context, accountID string) (bool, error)
	GetMembers(ctx context.Context, accountID string) ([]model.AccountMember, error)
	AddMember(ctx context.Context, m model.AccountMember) error
	UpdateMemberShare(ctx context.Context, accountID, userID string, share float64) error
	RemoveMember(ctx context.Context, accountID, userID string) error
}

type accountUserRepo interface {
	GetByUsername(ctx context.Context, username string) (model.User, error)
}

// accountTxRunner runs fn inside a DB transaction; fn receives a transaction-scoped
// accountRepo. Extracted as a function field so tests can supply a stub runner.
type accountTxRunner func(ctx context.Context, fn func(accountRepo) error) error

type AccountService struct {
	accounts accountRepo
	users    accountUserRepo
	withTx   accountTxRunner
}

func NewAccountService(pool *pgxpool.Pool, accounts *repository.AccountRepository, users *repository.UserRepository) *AccountService {
	return &AccountService{
		accounts: accounts,
		users:    users,
		withTx: func(ctx context.Context, fn func(accountRepo) error) error {
			return db.WithTx(ctx, pool, func(tx pgx.Tx) error {
				return fn(accounts.WithTx(tx))
			})
		},
	}
}

func (s *AccountService) ListByUser(ctx context.Context, userID string) ([]model.Account, error) {
	return s.accounts.ListByUser(ctx, userID)
}

func (s *AccountService) ListBalancesByUser(ctx context.Context, userID, displayCurrency string) (map[string]model.AccountBalance, error) {
	return s.accounts.ListBalancesByUser(ctx, userID, displayCurrency)
}

func (s *AccountService) GetByID(ctx context.Context, accountID string) (model.Account, error) {
	return s.accounts.GetByID(ctx, accountID)
}

func (s *AccountService) CreateAccount(ctx context.Context, ownerID string, req model.CreateAccountReq) (model.Account, error) {
	if req.AccessMode == model.AccountAccessModeShared && req.AcceptTransfers {
		return model.Account{}, fmt.Errorf("shared accounts accept transfers only from members: %w", apperr.ErrValidation)
	}
	members, err := s.creationMembers(ctx, ownerID, req)
	if err != nil {
		return model.Account{}, err
	}
	a := model.Account{
		OwnerID:            ownerID,
		AcceptTransfers:    req.AcceptTransfers,
		Name:               req.Name,
		AccessMode:         req.AccessMode,
		Kind:               req.Kind,
		Currency:           req.Currency,
		Icon:               req.Icon,
		InitialBalance:     req.InitialBalance,
		InitialBalanceDate: req.InitialBalanceDate,
	}

	var created model.Account
	err = s.withTx(ctx, func(accountsTx accountRepo) error {
		var innerErr error
		created, innerErr = accountsTx.Create(ctx, a)
		if innerErr != nil {
			return fmt.Errorf("create account: %w", innerErr)
		}

		for _, member := range members {
			member.AccountID = created.ID
			if innerErr = accountsTx.AddMember(ctx, member); innerErr != nil {
				return fmt.Errorf("add account member: %w", innerErr)
			}
		}
		return nil
	})
	if err != nil {
		return model.Account{}, err
	}
	return created, nil
}

func (s *AccountService) UpdateAccount(ctx context.Context, requesterID, accountID string, req model.UpdateAccountReq) (model.Account, error) {
	var updated model.Account
	err := s.withTx(ctx, func(accountsTx accountRepo) error {
		a, err := accountsTx.GetByID(ctx, accountID)
		if err != nil {
			return err
		}

		if req.AcceptTransfers != nil {
			if requesterID != a.OwnerID {
				return fmt.Errorf("only the owner can change transfer acceptance: %w", apperr.ErrForbidden)
			}
			a.AcceptTransfers = *req.AcceptTransfers
		}

		if req.AccessMode != nil {
			return immutableAccountError()
		}

		if a.AccessMode == model.AccountAccessModeShared {
			if req.AcceptTransfers != nil && *req.AcceptTransfers {
				return fmt.Errorf("shared accounts accept transfers only from members: %w", apperr.ErrValidation)
			}
			a.AcceptTransfers = false
		}
		if req.Name != nil {
			a.Name = strings.TrimSpace(*req.Name)
		}
		if req.Icon != nil {
			a.Icon = req.Icon
		}
		if req.InitialBalance != nil {
			a.InitialBalance = *req.InitialBalance
		}
		if req.InitialBalanceDate != nil {
			a.InitialBalanceDate = *req.InitialBalanceDate
		}

		updated, err = accountsTx.Update(ctx, a)
		if err != nil {
			return fmt.Errorf("update account: %w", err)
		}

		return nil
	})
	if err != nil {
		return model.Account{}, err
	}
	return updated, nil
}

func immutableAccountError() error {
	return fmt.Errorf("account mode, members and shares are immutable; create a new account and transfer funds: %w", apperr.ErrConflict)
}

// Resolve and validate the complete configuration before any database writes.
func (s *AccountService) creationMembers(ctx context.Context, ownerID string, req model.CreateAccountReq) ([]model.AccountMember, error) {
	if !req.AccessMode.IsValid() {
		return nil, fmt.Errorf("invalid account access mode: %w", apperr.ErrValidation)
	}
	if req.AccessMode == model.AccountAccessModePersonal && len(req.Members) == 0 {
		return []model.AccountMember{{UserID: ownerID, DefaultShare: 1}}, nil
	}
	if len(req.Members) == 0 {
		return nil, fmt.Errorf("members including the owner are required: %w", apperr.ErrValidation)
	}
	members := make([]model.AccountMember, 0, len(req.Members))
	seen := make(map[string]bool)
	total := 0
	for _, input := range req.Members {
		share := input.DefaultShare
		units := math.Round(share * 10000)
		if math.IsNaN(share) || math.IsInf(share, 0) || share < 0 || share > 1 || math.Abs(share*10000-units) > 1e-8 {
			return nil, fmt.Errorf("shares must be between 0 and 1 with at most four decimal places: %w", apperr.ErrValidation)
		}
		username := strings.TrimSpace(input.Username)
		if username == "" {
			return nil, fmt.Errorf("member username is required: %w", apperr.ErrValidation)
		}
		user, err := s.users.GetByUsername(ctx, username)
		if err != nil {
			if errors.Is(err, apperr.ErrNotFound) {
				return nil, fmt.Errorf("member %q not found: %w", username, apperr.ErrValidation)
			}
			return nil, fmt.Errorf("find account member: %w", err)
		}
		if !user.IsActive || seen[user.ID] {
			return nil, fmt.Errorf("members must be active and unique: %w", apperr.ErrValidation)
		}
		seen[user.ID] = true
		total += int(units)
		members = append(members, model.AccountMember{UserID: user.ID, Username: user.Username, DefaultShare: share})
	}
	if !seen[ownerID] || total != 10000 {
		return nil, fmt.Errorf("members must include the owner and shares must sum to 1: %w", apperr.ErrValidation)
	}
	if req.AccessMode == model.AccountAccessModePersonal && len(members) != 1 {
		return nil, fmt.Errorf("personal accounts belong only to their owner: %w", apperr.ErrValidation)
	}
	return members, nil
}

func (s *AccountService) DeleteAccount(ctx context.Context, requesterID, accountID string) error {
	a, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if a.OwnerID != requesterID {
		return fmt.Errorf("only the owner can delete an account: %w", apperr.ErrForbidden)
	}
	return s.accounts.SoftDelete(ctx, accountID)
}

// Legacy mutation endpoints remain callable but can no longer alter history.
func (s *AccountService) AddMember(_ context.Context, _, _ string, _ float64) ([]model.AccountMember, error) {
	return nil, immutableAccountError()
}

func (s *AccountService) UpdateMember(_ context.Context, _, _ string, _ float64) ([]model.AccountMember, error) {
	return nil, immutableAccountError()
}

func (s *AccountService) RemoveMember(_ context.Context, _, _, _ string) error {
	return immutableAccountError()
}

func (s *AccountService) GetMembers(ctx context.Context, accountID string) ([]model.AccountMember, error) {
	return s.accounts.GetMembers(ctx, accountID)
}

func (s *AccountService) ListTransferAccounts(ctx context.Context, username string) ([]model.TransferAccount, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 100 {
		return nil, fmt.Errorf("username is required: %w", apperr.ErrValidation)
	}
	return s.accounts.ListTransferAccounts(ctx, username)
}
