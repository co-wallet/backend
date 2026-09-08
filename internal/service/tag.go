package service

import (
	"context"
	"fmt"
	"github.com/co-wallet/backend/internal/apperr"
	"strings"
	"unicode/utf8"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

//go:generate mockgen -destination=mocks/mock_tag_repo.go -package=mocks github.com/co-wallet/backend/internal/service TagRepo
type TagRepo interface {
	Create(ctx context.Context, t model.Tag) (model.Tag, error)
	SetHidden(ctx context.Context, id, userID string, hidden bool) error
	ListByUser(ctx context.Context, userID string, q string) ([]model.TagWithCount, error)
	GetByID(ctx context.Context, id, userID string) (model.Tag, error)
	Update(ctx context.Context, t model.Tag) (model.Tag, error)
	Delete(ctx context.Context, id, userID string) error
	UpsertForTransaction(ctx context.Context, txID, userID string, names []string) ([]model.Tag, error)
	ListForTransaction(ctx context.Context, txID string) ([]model.Tag, error)
	ListForTransactions(ctx context.Context, txIDs []string) (map[string][]model.Tag, error)
}

type TagService struct {
	repo TagRepo
}

func NewTagService(repo *repository.TagRepository) *TagService {
	return &TagService{repo: repo}
}

func (s *TagService) List(ctx context.Context, userID, q string) ([]model.TagWithCount, error) {
	return s.repo.ListByUser(ctx, userID, q)
}

func (s *TagService) Rename(ctx context.Context, userID, id, name string) (model.Tag, error) {
	t, err := s.repo.GetByID(ctx, id, userID)
	if err != nil {
		return model.Tag{}, err
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || utf8.RuneCountInString(name) > 50 {
		return model.Tag{}, fmt.Errorf("tag name must contain 1–50 characters: %w", apperr.ErrValidation)
	}
	t.Name = name
	return s.repo.Update(ctx, t)
}

func (s *TagService) Delete(ctx context.Context, userID, id string) error {
	return s.repo.Delete(ctx, id, userID)
}

func (s *TagService) Create(ctx context.Context, userID, name string) (model.Tag, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || utf8.RuneCountInString(name) > 50 {
		return model.Tag{}, fmt.Errorf("tag name must contain 1–50 characters: %w", apperr.ErrValidation)
	}
	return s.repo.Create(ctx, model.Tag{UserID: userID, Name: name})
}

func (s *TagService) SetHidden(ctx context.Context, userID, id string, hidden bool) error {
	if _, err := s.repo.GetByID(ctx, id, userID); err != nil {
		return err
	}
	return s.repo.SetHidden(ctx, id, userID, hidden)
}
