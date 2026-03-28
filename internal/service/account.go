package service

import (
	"context"
	"fmt"

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

func (s *AccountService) CreateAccount(ctx context.Context, ownerID string, req model.CreateAccountReq) (*model.Account, error) {
	a := &model.Account{
		OwnerID:            ownerID,
		Name:               req.Name,
		Type:               req.Type,
		Currency:           req.Currency,
		Icon:               req.Icon,
		IncludeInBalance:   req.IncludeInBalance,
		InitialBalance:     req.InitialBalance,
		InitialBalanceDate: req.InitialBalanceDate,
	}
	if err := s.accounts.Create(ctx, a); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	// Auto-add owner as member of shared accounts with 100% share
	if a.Type == model.AccountTypeShared {
		if err := s.accounts.AddMember(ctx, model.AccountMember{
			AccountID:    a.ID,
			UserID:       ownerID,
			DefaultShare: 1.0,
		}); err != nil {
			return nil, fmt.Errorf("add owner as member: %w", err)
		}
	}

	return a, nil
}

func (s *AccountService) DeleteAccount(ctx context.Context, requesterID, accountID string) error {
	a, err := s.accounts.GetByID(ctx, accountID)
	if err != nil || a.DeletedAt != nil {
		return fmt.Errorf("account not found")
	}
	if a.OwnerID != requesterID {
		return fmt.Errorf("only the owner can delete an account")
	}
	return s.accounts.SoftDelete(ctx, accountID)
}

func (s *AccountService) AddMember(ctx context.Context, accountID, username string, share float64) ([]model.AccountMember, error) {
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("user not found")
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
		// Member was added; rollback to stay consistent
		_ = s.accounts.RemoveMember(ctx, accountID, u.ID)
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
		return fmt.Errorf("account not found")
	}
	if a.OwnerID != requesterID {
		return fmt.Errorf("only the owner can remove members")
	}
	if memberUserID == a.OwnerID {
		return fmt.Errorf("cannot remove the account owner")
	}
	return s.accounts.RemoveMember(ctx, accountID, memberUserID)
}
