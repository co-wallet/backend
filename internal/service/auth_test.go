package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service/mocks"
)

type AuthServiceSuite struct {
	suite.Suite
	ctrl *gomock.Controller
	repo *mocks.MockauthUserRepo
	svc  *AuthService
}

func (s *AuthServiceSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.repo = mocks.NewMockauthUserRepo(s.ctrl)
	s.svc = &AuthService{users: s.repo, jwtSecret: []byte("test-secret-xyz")}
}

func TestAuthServiceSuite(t *testing.T) {
	suite.Run(t, new(AuthServiceSuite))
}

func (s *AuthServiceSuite) activeUserWithPassword(password string) model.User {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	s.NoError(err)
	return model.User{
		ID:           "u1",
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: string(hash),
		IsActive:     true,
		IsAdmin:      false,
	}
}

func (s *AuthServiceSuite) TestLogin_Success() {
	u := s.activeUserWithPassword("secret123")
	s.repo.EXPECT().GetByEmail(gomock.Any(), "alice@example.com").Return(u, nil)

	got, tokens, err := s.svc.Login(context.Background(), "alice@example.com", "secret123")
	s.NoError(err)
	s.Equal("u1", got.ID)
	s.NotEmpty(tokens.AccessToken)
	s.NotEmpty(tokens.RefreshToken)
}

func (s *AuthServiceSuite) TestLogin_UserNotFound() {
	s.repo.EXPECT().GetByEmail(gomock.Any(), "x@x.com").Return(model.User{}, apperr.ErrNotFound)

	_, _, err := s.svc.Login(context.Background(), "x@x.com", "password1")
	s.True(errors.Is(err, apperr.ErrUnauthorized))
}

func (s *AuthServiceSuite) TestLogin_WrongPassword() {
	u := s.activeUserWithPassword("secret123")
	s.repo.EXPECT().GetByEmail(gomock.Any(), "alice@example.com").Return(u, nil)

	_, _, err := s.svc.Login(context.Background(), "alice@example.com", "wrong-password")
	s.True(errors.Is(err, apperr.ErrUnauthorized))
}

func (s *AuthServiceSuite) TestLogin_Inactive() {
	u := s.activeUserWithPassword("secret123")
	u.IsActive = false
	s.repo.EXPECT().GetByEmail(gomock.Any(), "alice@example.com").Return(u, nil)

	_, _, err := s.svc.Login(context.Background(), "alice@example.com", "secret123")
	s.True(errors.Is(err, apperr.ErrUnauthorized))
}

func (s *AuthServiceSuite) TestRefresh_Success() {
	u := s.activeUserWithPassword("secret123")
	tokens, err := s.svc.IssueTokens(u)
	s.NoError(err)

	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(u, nil)

	newTokens, err := s.svc.Refresh(context.Background(), tokens.RefreshToken)
	s.NoError(err)
	s.NotEmpty(newTokens.AccessToken)
}

func (s *AuthServiceSuite) TestRefresh_InvalidToken() {
	_, err := s.svc.Refresh(context.Background(), "garbage")
	s.True(errors.Is(err, apperr.ErrUnauthorized))
}

func (s *AuthServiceSuite) TestRefresh_UserNotFound() {
	u := s.activeUserWithPassword("secret123")
	tokens, err := s.svc.IssueTokens(u)
	s.NoError(err)

	s.repo.EXPECT().GetByID(gomock.Any(), "u1").Return(model.User{}, apperr.ErrNotFound)

	_, err = s.svc.Refresh(context.Background(), tokens.RefreshToken)
	s.True(errors.Is(err, apperr.ErrUnauthorized))
}

func (s *AuthServiceSuite) TestValidateAccessToken_Success() {
	u := s.activeUserWithPassword("secret123")
	tokens, err := s.svc.IssueTokens(u)
	s.NoError(err)

	claims, err := s.svc.ValidateAccessToken(tokens.AccessToken)
	s.NoError(err)
	s.Equal("u1", claims.UserID)
	s.False(claims.IsAdmin)
}

func (s *AuthServiceSuite) TestValidateAccessToken_Invalid() {
	_, err := s.svc.ValidateAccessToken("not.a.token")
	s.True(errors.Is(err, apperr.ErrUnauthorized))
}
