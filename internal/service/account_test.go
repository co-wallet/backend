package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/ptr"
	"github.com/co-wallet/backend/internal/service/mocks"
)

type AccountServiceSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	repo       *mocks.MockaccountRepo
	users      *mocks.MockaccountUserRepo
	svc        *AccountService
	txCommitCh bool
}

func TestAccountServiceSuite(t *testing.T) {
	suite.Run(t, new(AccountServiceSuite))
}

func (s *AccountServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockaccountRepo(s.ctrl)
	s.users = mocks.NewMockaccountUserRepo(s.ctrl)
	s.txCommitCh = false
	s.svc = &AccountService{
		accounts: s.repo,
		users:    s.users,
		withTx: func(ctx context.Context, fn func(accountRepo) error) error {
			err := fn(s.repo)
			if err == nil {
				s.txCommitCh = true
			}
			return err
		},
	}
}

func (s *AccountServiceSuite) TestCreateAccount_Personal_NoMemberAdded() {
	req := model.CreateAccountReq{
		Name:       "Wallet",
		AccessMode: model.AccountAccessModePersonal,
		Kind:       model.AccountKindSpending,
		Currency:   "USD",
	}
	s.repo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, a model.Account) (model.Account, error) {
			s.Equal("owner-1", a.OwnerID)
			s.Equal(model.AccountAccessModePersonal, a.AccessMode)
			s.Equal(model.AccountKindSpending, a.Kind)
			a.ID = "acc-1"
			return a, nil
		})
	// Personal accounts must NOT trigger AddMember — no expectation means 0 calls expected

	acc, err := s.svc.CreateAccount(context.Background(), "owner-1", req)
	s.NoError(err)
	s.Equal("acc-1", acc.ID)
	s.True(s.txCommitCh, "tx should commit on success")
}

func (s *AccountServiceSuite) TestCreateAccount_Shared_AddsOwnerAsMember() {
	req := model.CreateAccountReq{
		Name:       "Family",
		AccessMode: model.AccountAccessModeShared,
		Kind:       model.AccountKindSpending,
		Currency:   "USD",
	}
	gomock.InOrder(
		s.repo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, a model.Account) (model.Account, error) {
				a.ID = "acc-1"
				s.Equal(model.AccountAccessModeShared, a.AccessMode)
				return a, nil
			}),
		s.repo.EXPECT().
			AddMember(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, m model.AccountMember) error {
				s.Equal("acc-1", m.AccountID)
				s.Equal("owner-1", m.UserID)
				s.Equal(1.0, m.DefaultShare)
				return nil
			}),
	)

	_, err := s.svc.CreateAccount(context.Background(), "owner-1", req)
	s.NoError(err)
	s.True(s.txCommitCh)
}

func (s *AccountServiceSuite) TestCreateAccount_AddMemberFailureRollsBack() {
	req := model.CreateAccountReq{Name: "F", AccessMode: model.AccountAccessModeShared, Kind: model.AccountKindSpending, Currency: "USD"}
	gomock.InOrder(
		s.repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(model.Account{ID: "a", AccessMode: model.AccountAccessModeShared}, nil),
		s.repo.EXPECT().AddMember(gomock.Any(), gomock.Any()).Return(errors.New("dup")),
	)

	_, err := s.svc.CreateAccount(context.Background(), "owner-1", req)
	s.Error(err)
	s.False(s.txCommitCh, "tx must not commit when AddMember fails")
}

func (s *AccountServiceSuite) TestUpdateAccount_AppliesPartialPatch() {
	existing := model.Account{
		ID: "acc-1", OwnerID: "owner-1", Name: "Old",
		Kind: model.AccountKindInvestment, InitialBalance: 100,
	}
	s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(existing, nil)
	s.repo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, a model.Account) (model.Account, error) {
			s.Equal("New", a.Name)
			s.Equal(model.AccountKindInvestment, a.Kind)
			s.Equal(100.0, a.InitialBalance)
			return a, nil
		})

	_, err := s.svc.UpdateAccount(context.Background(), "member-1", "acc-1", model.UpdateAccountReq{
		Name: ptr.To("  New  "),
	})
	s.NoError(err)
}

func (s *AccountServiceSuite) TestUpdateAccount_PersonalToSharedAddsOwner() {
	existing := model.Account{
		ID: "acc-1", OwnerID: "owner-1", Name: "Wallet", AccessMode: model.AccountAccessModePersonal,
	}
	gomock.InOrder(
		s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(existing, nil),
		s.repo.EXPECT().
			Update(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, a model.Account) (model.Account, error) {
				s.Equal(model.AccountAccessModeShared, a.AccessMode)
				return a, nil
			}),
		s.repo.EXPECT().
			AddMember(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, member model.AccountMember) error {
				s.Equal("acc-1", member.AccountID)
				s.Equal("owner-1", member.UserID)
				s.Equal(1.0, member.DefaultShare)
				return nil
			}),
	)

	updated, err := s.svc.UpdateAccount(context.Background(), "owner-1", "acc-1", model.UpdateAccountReq{
		AccessMode: ptr.To(model.AccountAccessModeShared),
	})

	s.NoError(err)
	s.Equal(model.AccountAccessModeShared, updated.AccessMode)
	s.True(s.txCommitCh)
}

func (s *AccountServiceSuite) TestUpdateAccount_AccessModeChangeByNonOwnerForbidden() {
	s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(model.Account{
		ID: "acc-1", OwnerID: "owner-1", AccessMode: model.AccountAccessModePersonal,
	}, nil)

	_, err := s.svc.UpdateAccount(context.Background(), "member-1", "acc-1", model.UpdateAccountReq{
		AccessMode: ptr.To(model.AccountAccessModeShared),
	})

	s.ErrorIs(err, apperr.ErrForbidden)
	s.False(s.txCommitCh)
}

func (s *AccountServiceSuite) TestUpdateAccount_SharedToPersonalWithOtherMembersConflicts() {
	existing := model.Account{ID: "acc-1", OwnerID: "owner-1", AccessMode: model.AccountAccessModeShared}
	s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(existing, nil)
	s.repo.EXPECT().GetMembers(gomock.Any(), "acc-1").Return([]model.AccountMember{
		{AccountID: "acc-1", UserID: "owner-1"},
		{AccountID: "acc-1", UserID: "member-1"},
	}, nil)

	_, err := s.svc.UpdateAccount(context.Background(), "owner-1", "acc-1", model.UpdateAccountReq{
		AccessMode: ptr.To(model.AccountAccessModePersonal),
	})

	s.ErrorIs(err, apperr.ErrConflict)
	s.False(s.txCommitCh)
}

func (s *AccountServiceSuite) TestUpdateAccount_SharedToPersonalWithTransactionsConflicts() {
	existing := model.Account{ID: "acc-1", OwnerID: "owner-1", AccessMode: model.AccountAccessModeShared}
	s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(existing, nil)
	s.repo.EXPECT().GetMembers(gomock.Any(), "acc-1").Return([]model.AccountMember{
		{AccountID: "acc-1", UserID: "owner-1"},
	}, nil)
	s.repo.EXPECT().HasTransactions(gomock.Any(), "acc-1").Return(true, nil)

	_, err := s.svc.UpdateAccount(context.Background(), "owner-1", "acc-1", model.UpdateAccountReq{
		AccessMode: ptr.To(model.AccountAccessModePersonal),
	})

	s.ErrorIs(err, apperr.ErrConflict)
	s.False(s.txCommitCh)
}

func (s *AccountServiceSuite) TestUpdateAccount_SharedToPersonalRemovesOwnerMembership() {
	existing := model.Account{ID: "acc-1", OwnerID: "owner-1", AccessMode: model.AccountAccessModeShared}
	gomock.InOrder(
		s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(existing, nil),
		s.repo.EXPECT().GetMembers(gomock.Any(), "acc-1").Return([]model.AccountMember{
			{AccountID: "acc-1", UserID: "owner-1"},
		}, nil),
		s.repo.EXPECT().HasTransactions(gomock.Any(), "acc-1").Return(false, nil),
		s.repo.EXPECT().
			Update(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, a model.Account) (model.Account, error) {
				s.Equal(model.AccountAccessModePersonal, a.AccessMode)
				return a, nil
			}),
		s.repo.EXPECT().RemoveMember(gomock.Any(), "acc-1", "owner-1").Return(nil),
	)

	updated, err := s.svc.UpdateAccount(context.Background(), "owner-1", "acc-1", model.UpdateAccountReq{
		AccessMode: ptr.To(model.AccountAccessModePersonal),
	})

	s.NoError(err)
	s.Equal(model.AccountAccessModePersonal, updated.AccessMode)
	s.True(s.txCommitCh)
}

func (s *AccountServiceSuite) TestDeleteAccount_NotOwnerForbidden() {
	s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(model.Account{ID: "acc-1", OwnerID: "someone-else"}, nil)

	err := s.svc.DeleteAccount(context.Background(), "requester", "acc-1")
	s.True(errors.Is(err, apperr.ErrForbidden))
}

func (s *AccountServiceSuite) TestDeleteAccount_OwnerSoftDeletes() {
	s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(model.Account{ID: "acc-1", OwnerID: "owner-1"}, nil)
	s.repo.EXPECT().SoftDelete(gomock.Any(), "acc-1").Return(nil)

	err := s.svc.DeleteAccount(context.Background(), "owner-1", "acc-1")
	s.NoError(err)
}

func (s *AccountServiceSuite) TestAddMember_UserNotFound() {
	s.users.EXPECT().GetByUsername(gomock.Any(), "ghost").Return(model.User{}, errors.New("no user"))

	_, err := s.svc.AddMember(context.Background(), "acc-1", "ghost", 0.5)
	s.True(errors.Is(err, apperr.ErrNotFound))
}

func (s *AccountServiceSuite) TestAddMember_Success() {
	s.users.EXPECT().GetByUsername(gomock.Any(), "bob").Return(model.User{ID: "bob-id"}, nil)
	s.repo.EXPECT().
		AddMember(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, m model.AccountMember) error {
			s.Equal("acc-1", m.AccountID)
			s.Equal("bob-id", m.UserID)
			s.Equal(0.5, m.DefaultShare)
			return nil
		})
	s.repo.EXPECT().GetMembers(gomock.Any(), "acc-1").Return([]model.AccountMember{
		{AccountID: "acc-1", UserID: "owner", DefaultShare: 0.5},
		{AccountID: "acc-1", UserID: "bob-id", DefaultShare: 0.5},
	}, nil)

	members, err := s.svc.AddMember(context.Background(), "acc-1", "bob", 0.5)
	s.NoError(err)
	s.Len(members, 2)
}

func (s *AccountServiceSuite) TestRemoveMember_NotOwnerForbidden() {
	s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(model.Account{ID: "acc-1", OwnerID: "owner-1"}, nil)

	err := s.svc.RemoveMember(context.Background(), "requester", "acc-1", "bob")
	s.True(errors.Is(err, apperr.ErrForbidden))
}

func (s *AccountServiceSuite) TestRemoveMember_CannotRemoveOwner() {
	s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(model.Account{ID: "acc-1", OwnerID: "owner-1"}, nil)

	err := s.svc.RemoveMember(context.Background(), "owner-1", "acc-1", "owner-1")
	s.True(errors.Is(err, apperr.ErrForbidden))
}

func (s *AccountServiceSuite) TestRemoveMember_OwnerRemovesOther() {
	s.repo.EXPECT().GetByID(gomock.Any(), "acc-1").Return(model.Account{ID: "acc-1", OwnerID: "owner-1"}, nil)
	s.repo.EXPECT().RemoveMember(gomock.Any(), "acc-1", "bob").Return(nil)

	err := s.svc.RemoveMember(context.Background(), "owner-1", "acc-1", "bob")
	s.NoError(err)
}

func (s *AccountServiceSuite) TestUpdateMember_RefetchesMembers() {
	s.repo.EXPECT().UpdateMemberShare(gomock.Any(), "acc-1", "bob", 0.3).Return(nil)
	s.repo.EXPECT().GetMembers(gomock.Any(), "acc-1").Return([]model.AccountMember{
		{AccountID: "acc-1", UserID: "bob", DefaultShare: 0.3},
	}, nil)

	members, err := s.svc.UpdateMember(context.Background(), "acc-1", "bob", 0.3)
	s.NoError(err)
	s.Len(members, 1)
	s.Equal(0.3, members[0].DefaultShare)
}
