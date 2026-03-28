package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
)

type inviteRepo interface {
	Create(ctx context.Context, inv model.Invite) error
	GetByToken(ctx context.Context, token string) (*model.Invite, error)
	MarkUsed(ctx context.Context, token string) error
	ListAll(ctx context.Context) ([]model.Invite, error)
}

type inviteUserRepo interface {
	Create(ctx context.Context, u *model.User) error
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
}

type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

type InviteService struct {
	repo     inviteRepo
	users    inviteUserRepo
	auth     *AuthService
	smtp     SMTPConfig
	appURL   string
}

func NewInviteService(repo inviteRepo, users inviteUserRepo, auth *AuthService, smtp SMTPConfig, appURL string) *InviteService {
	return &InviteService{repo: repo, users: users, auth: auth, smtp: smtp, appURL: appURL}
}

// CreateInvite generates an invite token, stores it, and optionally sends an email.
// Returns the full invite URL so the admin can share it manually if SMTP is not configured.
func (s *InviteService) CreateInvite(ctx context.Context, email, createdBy string) (model.Invite, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return model.Invite{}, "", fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(b)

	inv := model.Invite{
		Email:     strings.ToLower(strings.TrimSpace(email)),
		Token:     token,
		CreatedBy: createdBy,
		ExpiresAt: time.Now().Add(72 * time.Hour),
	}
	if err := s.repo.Create(ctx, inv); err != nil {
		return model.Invite{}, "", fmt.Errorf("save invite: %w", err)
	}

	inviteURL := fmt.Sprintf("%s/invite/%s", strings.TrimRight(s.appURL, "/"), token)

	if s.smtp.Host != "" {
		_ = s.sendEmail(inv.Email, inviteURL) // best-effort — don't fail if email bounces
	}

	return inv, inviteURL, nil
}

func (s *InviteService) sendEmail(to, inviteURL string) error {
	subject := "Приглашение в co-wallet"
	body := fmt.Sprintf(
		"Вас пригласили в co-wallet.\r\n\r\nПерейдите по ссылке для создания аккаунта:\r\n%s\r\n\r\nСсылка действительна 72 часа.",
		inviteURL,
	)
	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		s.smtp.From, to, subject, body,
	))

	addr := s.smtp.Host + ":" + s.smtp.Port
	var auth smtp.Auth
	if s.smtp.User != "" {
		auth = smtp.PlainAuth("", s.smtp.User, s.smtp.Pass, s.smtp.Host)
	}
	return smtp.SendMail(addr, auth, s.smtp.From, []string{to}, msg)
}

func (s *InviteService) ListInvites(ctx context.Context) ([]model.Invite, error) {
	invites, err := s.repo.ListAll(ctx)
	if invites == nil {
		invites = []model.Invite{}
	}
	return invites, err
}

// ValidateToken returns the invite if the token is valid (not expired, not used).
func (s *InviteService) ValidateToken(ctx context.Context, token string) (*model.Invite, error) {
	inv, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid invite link", apperr.ErrNotFound)
	}
	if inv.UsedAt != nil {
		return nil, fmt.Errorf("%w: invite already used", apperr.ErrValidation)
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, fmt.Errorf("%w: invite link has expired", apperr.ErrValidation)
	}
	return inv, nil
}

type AcceptInviteReq struct {
	Token           string `json:"token"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	DefaultCurrency string `json:"defaultCurrency"`
}

func (s *InviteService) AcceptInvite(ctx context.Context, req AcceptInviteReq) (*model.User, *TokenPair, error) {
	inv, err := s.ValidateToken(ctx, req.Token)
	if err != nil {
		return nil, nil, err
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		return nil, nil, fmt.Errorf("%w: username and password (min 8 chars) required", apperr.ErrValidation)
	}

	currency := strings.ToUpper(strings.TrimSpace(req.DefaultCurrency))
	if currency == "" {
		currency = "USD"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}

	u := &model.User{
		Username:        req.Username,
		Email:           inv.Email,
		PasswordHash:    string(hash),
		DefaultCurrency: currency,
		IsAdmin:         false,
		IsActive:        true,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, nil, fmt.Errorf("%w: username or email already taken", apperr.ErrConflict)
	}

	if err := s.repo.MarkUsed(ctx, req.Token); err != nil {
		return nil, nil, fmt.Errorf("mark invite used: %w", err)
	}

	tokens, err := s.auth.IssueTokens(u)
	if err != nil {
		return nil, nil, err
	}

	return u, tokens, nil
}
