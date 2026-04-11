package service

import (
	"context"
	"fmt"
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
	ListByUser(ctx context.Context, userID string) ([]model.Account, error)
	ListBalancesByUser(ctx context.Context, userID, displayCurrency string) (map[string]model.AccountBalance, error)
	GetByID(ctx context.Context, id string) (model.Account, error)
	Create(ctx context.Context, a model.Account) (model.Account, error)
	Update(ctx context.Context, a model.Account) (model.Account, error)
	SoftDelete(ctx context.Context, id string) error
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
	a := model.Account{
		OwnerID:            ownerID,
		Name:               req.Name,
		Type:               req.Type,
		Currency:           req.Currency,
		Icon:               req.Icon,
		IncludeInBalance:   req.IncludeInBalance,
		InitialBalance:     req.InitialBalance,
		InitialBalanceDate: req.InitialBalanceDate,
	}

	var created model.Account
	err := s.withTx(ctx, func(accountsTx accountRepo) error {
		var innerErr error
		created, innerErr = accountsTx.Create(ctx, a)
		if innerErr != nil {
			return fmt.Errorf("create account: %w", innerErr)
		}

		if created.Type == model.AccountTypeShared {
			if innerErr = accountsTx.AddMember(ctx, model.AccountMember{
				AccountID:    created.ID,
				UserID:       ownerID,
				DefaultShare: 1.0,
			}); innerErr != nil {
				return fmt.Errorf("add owner as member: %w", innerErr)
			}
		}
		return nil
	})
	if err != nil {
		return model.Account{}, err
	}
	return created, nil
}

func (s *AccountService) UpdateAccount(ctx context.Context, accountID string, req model.UpdateAccountReq) (model.Account, error) {
	a, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return model.Account{}, err
	}
	if req.Name != nil {
		a.Name = strings.TrimSpace(*req.Name)
	}
	if req.Icon != nil {
		a.Icon = req.Icon
	}
	if req.IncludeInBalance != nil {
		a.IncludeInBalance = *req.IncludeInBalance
	}
	if req.InitialBalance != nil {
		a.InitialBalance = *req.InitialBalance
	}
	if req.InitialBalanceDate != nil {
		a.InitialBalanceDate = *req.InitialBalanceDate
	}

	return s.accounts.Update(ctx, a)
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

func (s *AccountService) AddMember(ctx context.Context, accountID, username string, share float64) ([]model.AccountMember, error) {
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("user %q: %w", username, apperr.ErrNotFound)
	}

	if err := s.accounts.AddMember(ctx, model.AccountMember{
		AccountID:    accountID,
		UserID:       u.ID,
		DefaultShare: share,
	}); err != nil {
		return nil, fmt.Errorf("add member: %w", err)
	}

	members, err := s.accounts.GetMembers(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("fetch members after add: %w", err)
	}
	return members, nil
}

func (s *AccountService) UpdateMember(ctx context.Context, accountID, memberUserID string, share float64) ([]model.AccountMember, error) {
	if err := s.accounts.UpdateMemberShare(ctx, accountID, memberUserID, share); err != nil {
		return nil, fmt.Errorf("update member share: %w", err)
	}
	members, err := s.accounts.GetMembers(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("fetch members after update: %w", err)
	}
	return members, nil
}

func (s *AccountService) RemoveMember(ctx context.Context, requesterID, accountID, memberUserID string) error {
	a, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if a.OwnerID != requesterID {
		return fmt.Errorf("only the owner can remove members: %w", apperr.ErrForbidden)
	}
	if memberUserID == a.OwnerID {
		return fmt.Errorf("cannot remove the account owner: %w", apperr.ErrForbidden)
	}
	return s.accounts.RemoveMember(ctx, accountID, memberUserID)
}

func (s *AccountService) GetMembers(ctx context.Context, accountID string) ([]model.AccountMember, error) {
	return s.accounts.GetMembers(ctx, accountID)
}
