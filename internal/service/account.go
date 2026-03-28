package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

type AccountService struct {
	accounts *repository.AccountRepository
	users    *repository.UserRepository
}

func NewAccountService(accounts *repository.AccountRepository, users *repository.UserRepository) *AccountService {
	return &AccountService{accounts: accounts, users: users}
}

func (s *AccountService) ListByUser(ctx context.Context, userID string) ([]model.Account, error) {
	return s.accounts.ListByUser(ctx, userID)
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
	created, err := s.accounts.Create(ctx, a)
	if err != nil {
		return model.Account{}, fmt.Errorf("create account: %w", err)
	}

	// Auto-add owner as member of shared accounts with 100% share
	if created.Type == model.AccountTypeShared {
		if err := s.accounts.AddMember(ctx, model.AccountMember{
			AccountID:    created.ID,
			UserID:       ownerID,
			DefaultShare: 1.0,
		}); err != nil {
			return model.Account{}, fmt.Errorf("add owner as member: %w", err)
		}
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
	return s.accounts.Update(ctx, a)
}

func (s *AccountService) DeleteAccount(ctx context.Context, requesterID, accountID string) error {
	a, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return err // already wrapped with ErrNotFound from repo
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
		if rollbackErr := s.accounts.RemoveMember(ctx, accountID, u.ID); rollbackErr != nil {
			log.Printf("rollback RemoveMember failed for account=%s user=%s: %v", accountID, u.ID, rollbackErr)
		}
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
