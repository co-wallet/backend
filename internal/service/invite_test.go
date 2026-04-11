package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service/mocks"
)

type InviteServiceSuite struct {
	suite.Suite
	ctrl    *gomock.Controller
	repo    *mocks.MockinviteRepo
	users   *mocks.MockinviteUserRepo
	auth    *AuthService
	svc     *InviteService
	runTxFn inviteTxRunner
}

func TestInviteServiceSuite(t *testing.T) {
	suite.Run(t, new(InviteServiceSuite))
}

func (s *InviteServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockinviteRepo(s.ctrl)
	s.users = mocks.NewMockinviteUserRepo(s.ctrl)
	s.auth = &AuthService{jwtSecret: []byte("test-secret-xyz")}
	// Stubbed tx runner: invoke fn directly with the mock repos, no real DB involved.
	s.runTxFn = func(ctx context.Context, fn func(inviteRepo, inviteUserRepo) error) error {
		return fn(s.repo, s.users)
	}
	s.svc = &InviteService{
		repo:   s.repo,
		users:  s.users,
		auth:   s.auth,
		appURL: "https://cowallet.test",
		withTx: s.runTxFn,
	}
}

func (s *InviteServiceSuite) TestCreateInvite_Success() {
	s.repo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, inv model.Invite) error {
			s.Equal("alice@example.com", inv.Email)
			s.Equal("admin-1", inv.CreatedBy)
			s.Len(inv.Token, 64) // 32 bytes hex-encoded
			s.True(inv.ExpiresAt.After(time.Now().Add(71 * time.Hour)))
			return nil
		})

	inv, url, err := s.svc.CreateInvite(context.Background(), "  Alice@Example.com ", "admin-1")
	s.NoError(err)
	s.Equal("alice@example.com", inv.Email)
	s.Contains(url, "https://cowallet.test/invite/")
}

func (s *InviteServiceSuite) TestCreateInvite_RepoError() {
	s.repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(errors.New("db down"))

	_, _, err := s.svc.CreateInvite(context.Background(), "alice@example.com", "admin-1")
	s.Error(err)
}

func (s *InviteServiceSuite) TestValidateToken_NotFound() {
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(nil, errors.New("not found"))

	_, err := s.svc.ValidateToken(context.Background(), "tok")
	s.True(errors.Is(err, apperr.ErrNotFound))
}

func (s *InviteServiceSuite) TestValidateToken_AlreadyUsed() {
	used := time.Now().Add(-time.Hour)
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(&model.Invite{
		Token:     "tok",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		UsedAt:    &used,
	}, nil)

	_, err := s.svc.ValidateToken(context.Background(), "tok")
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *InviteServiceSuite) TestValidateToken_Expired() {
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(&model.Invite{
		Token:     "tok",
		ExpiresAt: time.Now().Add(-time.Hour),
	}, nil)

	_, err := s.svc.ValidateToken(context.Background(), "tok")
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *InviteServiceSuite) TestValidateToken_Valid() {
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(&model.Invite{
		Token:     "tok",
		Email:     "alice@example.com",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}, nil)

	inv, err := s.svc.ValidateToken(context.Background(), "tok")
	s.NoError(err)
	s.Equal("alice@example.com", inv.Email)
}

func (s *InviteServiceSuite) validInvite() *model.Invite {
	return &model.Invite{
		Token:     "tok",
		Email:     "alice@example.com",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
}

func (s *InviteServiceSuite) TestAcceptInvite_Success() {
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(s.validInvite(), nil)
	s.users.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, u model.User) (model.User, error) {
			s.Equal("alice@example.com", u.Email)
			s.Equal("alice", u.Username)
			s.Equal("EUR", u.DefaultCurrency)
			s.True(u.IsActive)
			s.False(u.IsAdmin)
			u.ID = "user-created"
			return u, nil
		})
	s.repo.EXPECT().MarkUsed(gomock.Any(), "tok").Return(nil)

	u, tokens, err := s.svc.AcceptInvite(context.Background(), AcceptInviteReq{
		Token:           "tok",
		Username:        "  alice  ",
		Password:        "strong-password",
		DefaultCurrency: "eur",
	})
	s.NoError(err)
	s.Equal("user-created", u.ID)
	s.NotEmpty(tokens.AccessToken)
	s.NotEmpty(tokens.RefreshToken)
}

func (s *InviteServiceSuite) TestAcceptInvite_DefaultCurrencyFallsBackToUSD() {
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(s.validInvite(), nil)
	s.users.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, u model.User) (model.User, error) {
			s.Equal("USD", u.DefaultCurrency)
			u.ID = "u"
			return u, nil
		})
	s.repo.EXPECT().MarkUsed(gomock.Any(), "tok").Return(nil)

	_, _, err := s.svc.AcceptInvite(context.Background(), AcceptInviteReq{
		Token: "tok", Username: "alice", Password: "strong-password",
	})
	s.NoError(err)
}

func (s *InviteServiceSuite) TestAcceptInvite_WeakPassword() {
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(s.validInvite(), nil)

	_, _, err := s.svc.AcceptInvite(context.Background(), AcceptInviteReq{
		Token: "tok", Username: "alice", Password: "short",
	})
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *InviteServiceSuite) TestAcceptInvite_EmptyUsername() {
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(s.validInvite(), nil)

	_, _, err := s.svc.AcceptInvite(context.Background(), AcceptInviteReq{
		Token: "tok", Username: "   ", Password: "strong-password",
	})
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *InviteServiceSuite) TestAcceptInvite_DuplicateUser() {
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(s.validInvite(), nil)
	s.users.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(model.User{}, errors.New("unique violation"))

	_, _, err := s.svc.AcceptInvite(context.Background(), AcceptInviteReq{
		Token: "tok", Username: "alice", Password: "strong-password",
	})
	s.True(errors.Is(err, apperr.ErrConflict))
}

func (s *InviteServiceSuite) TestAcceptInvite_InvalidToken() {
	used := time.Now().Add(-time.Hour)
	inv := s.validInvite()
	inv.UsedAt = &used
	s.repo.EXPECT().GetByToken(gomock.Any(), "tok").Return(inv, nil)

	_, _, err := s.svc.AcceptInvite(context.Background(), AcceptInviteReq{
		Token: "tok", Username: "alice", Password: "strong-password",
	})
	s.True(errors.Is(err, apperr.ErrValidation))
}

func (s *InviteServiceSuite) TestListInvites_NilNormalizedToEmpty() {
	s.repo.EXPECT().ListAll(gomock.Any()).Return(nil, nil)

	got, err := s.svc.ListInvites(context.Background())
	s.NoError(err)
	s.NotNil(got)
	s.Empty(got)
}

