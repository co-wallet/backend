package service

import (
	"context"
	"errors"
	"math"
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

func (s *AccountServiceSuite) TestCreateAccount_Personal_OwnerHasFullShare() {
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
	s.repo.EXPECT().AddMember(gomock.Any(), model.AccountMember{AccountID: "acc-1", UserID: "owner-1", DefaultShare: 1}).Return(nil)

	acc, err := s.svc.CreateAccount(context.Background(), "owner-1", req)
	s.NoError(err)
	s.Equal("acc-1", acc.ID)
	s.True(s.txCommitCh, "tx should commit on success")
}

func (s *AccountServiceSuite) TestCreateAccount_Shared_AddsOwnerAsMember() {
	req := model.CreateAccountReq{
		Members:    []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: 1}},
		Name:       "Family",
		AccessMode: model.AccountAccessModeShared,
		Kind:       model.AccountKindSpending,
		Currency:   "USD",
	}
	s.users.EXPECT().GetByUsername(gomock.Any(), "owner").Return(model.User{ID: "owner-1", IsActive: true}, nil)
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
	req := model.CreateAccountReq{Name: "F", AccessMode: model.AccountAccessModePersonal, Kind: model.AccountKindSpending, Currency: "USD"}
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

func (s *AccountServiceSuite) TestConfigurationChangesRejected() {
	for _, mode := range []model.AccountAccessMode{model.AccountAccessModePersonal, model.AccountAccessModeShared} {
		s.repo.EXPECT().GetByID(gomock.Any(), "a").Return(model.Account{ID: "a", AccessMode: mode}, nil)
		_, err := s.svc.UpdateAccount(context.Background(), "owner", "a", model.UpdateAccountReq{AccessMode: ptr.To(mode)})
		s.ErrorIs(err, apperr.ErrConflict)
	}
	_, err := s.svc.AddMember(context.Background(), "a", "bob", 0.5)
	s.ErrorIs(err, apperr.ErrConflict)
	_, err = s.svc.UpdateMember(context.Background(), "a", "bob", 0.5)
	s.ErrorIs(err, apperr.ErrConflict)
	s.ErrorIs(s.svc.RemoveMember(context.Background(), "owner", "a", "bob"), apperr.ErrConflict)
	s.False(s.txCommitCh)
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

func (s *AccountServiceSuite) TestTransferAcceptanceOwnerOnly() {
	s.repo.EXPECT().GetByID(gomock.Any(), "a").Return(model.Account{ID: "a", OwnerID: "owner"}, nil)
	_, err := s.svc.UpdateAccount(context.Background(), "member", "a", model.UpdateAccountReq{AcceptTransfers: ptr.To(true)})
	s.ErrorIs(err, apperr.ErrForbidden)
}
func (s *AccountServiceSuite) TestTransferAcceptanceOwnerUpdate() {
	s.repo.EXPECT().GetByID(gomock.Any(), "a").Return(model.Account{ID: "a", OwnerID: "owner"}, nil)
	s.repo.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, a model.Account) (model.Account, error) {
		s.True(a.AcceptTransfers)
		return a, nil
	})
	_, err := s.svc.UpdateAccount(context.Background(), "owner", "a", model.UpdateAccountReq{AcceptTransfers: ptr.To(true)})
	s.NoError(err)
}
func (s *AccountServiceSuite) TestTransferSearch() {
	_, err := s.svc.ListTransferAccounts(context.Background(), " ")
	s.ErrorIs(err, apperr.ErrValidation)
	s.repo.EXPECT().ListTransferAccounts(gomock.Any(), "alice").Return([]model.TransferAccount{{ID: "open"}}, nil)
	accounts, err := s.svc.ListTransferAccounts(context.Background(), " alice ")
	s.NoError(err)
	s.Len(accounts, 1)
}

func (s *AccountServiceSuite) TestSharedAccountCannotEnableExternalTransfers() {
	_, err := s.svc.CreateAccount(context.Background(), "owner", model.CreateAccountReq{AccessMode: model.AccountAccessModeShared, AcceptTransfers: true})
	s.ErrorIs(err, apperr.ErrValidation)
	s.repo.EXPECT().GetByID(gomock.Any(), "a").Return(model.Account{ID: "a", OwnerID: "owner", AccessMode: model.AccountAccessModeShared}, nil)
	_, err = s.svc.UpdateAccount(context.Background(), "owner", "a", model.UpdateAccountReq{AcceptTransfers: ptr.To(true)})
	s.ErrorIs(err, apperr.ErrValidation)
}

func (s *AccountServiceSuite) TestCreateSharedAccount_CompleteConfiguration() {
	s.users.EXPECT().GetByUsername(gomock.Any(), "owner").Return(model.User{ID: "owner", Username: "owner", IsActive: true}, nil)
	s.users.EXPECT().GetByUsername(gomock.Any(), "bob").Return(model.User{ID: "bob", Username: "bob", IsActive: true}, nil)
	s.repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(model.Account{ID: "a", AccessMode: model.AccountAccessModeShared}, nil)
	s.repo.EXPECT().AddMember(gomock.Any(), model.AccountMember{AccountID: "a", UserID: "owner", Username: "owner", DefaultShare: 0.3333}).Return(nil)
	s.repo.EXPECT().AddMember(gomock.Any(), model.AccountMember{AccountID: "a", UserID: "bob", Username: "bob", DefaultShare: 0.6667}).Return(nil)
	_, err := s.svc.CreateAccount(context.Background(), "owner", model.CreateAccountReq{AccessMode: model.AccountAccessModeShared, Members: []model.CreateAccountMemberReq{{Username: " owner ", DefaultShare: 0.3333}, {Username: "bob", DefaultShare: 0.6667}}})
	s.NoError(err)
	s.True(s.txCommitCh)
}

func (s *AccountServiceSuite) TestCreateAccount_InvalidConfigurations() {
	cases := []struct {
		name    string
		mode    model.AccountAccessMode
		members []model.CreateAccountMemberReq
	}{
		{"empty shared", model.AccountAccessModeShared, nil},
		{"missing owner", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "bob", DefaultShare: 1}}},
		{"duplicate", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: 0.5}, {Username: "owner", DefaultShare: 0.5}}},
		{"sum", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: 0.9}}},
		{"precision", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: 0.99999}}},
		{"negative", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: -0.1}}},
		{"above one", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: 1.1}}},
		{"nan", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: math.NaN()}}},
		{"infinite", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: math.Inf(1)}}},
		{"empty username", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{DefaultShare: 1}}},
		{"personal extra", model.AccountAccessModePersonal, []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: 0.5}, {Username: "bob", DefaultShare: 0.5}}},
		{"inactive", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "inactive", DefaultShare: 1}}},
		{"unknown", model.AccountAccessModeShared, []model.CreateAccountMemberReq{{Username: "unknown", DefaultShare: 1}}},
	}
	for _, tt := range cases {
		s.Run(tt.name, func() {
			s.SetupTest()
			s.users.EXPECT().GetByUsername(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, username string) (model.User, error) {
				if username == "unknown" {
					return model.User{}, apperr.ErrNotFound
				}
				return model.User{ID: username, Username: username, IsActive: username != "inactive"}, nil
			}).AnyTimes()
			_, err := s.svc.CreateAccount(context.Background(), "owner", model.CreateAccountReq{AccessMode: tt.mode, Members: tt.members})
			s.ErrorIs(err, apperr.ErrValidation)
			s.False(s.txCommitCh)
		})
	}
}

func (s *AccountServiceSuite) TestCreateAccount_UserLookupFailure() {
	failure := errors.New("database unavailable")
	s.users.EXPECT().GetByUsername(gomock.Any(), "owner").Return(model.User{}, failure)
	_, err := s.svc.CreateAccount(context.Background(), "owner", model.CreateAccountReq{AccessMode: model.AccountAccessModeShared, Members: []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: 1}}})
	s.ErrorIs(err, failure)
	s.False(s.txCommitCh)
}
