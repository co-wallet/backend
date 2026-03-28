package service

import (
	"context"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

//go:generate mockgen -destination=mocks/mock_tag_repo.go -package=mocks github.com/co-wallet/backend/internal/service TagRepo
type TagRepo interface {
	ListByUser(ctx context.Context, userID string, q string) ([]model.TagWithCount, error)
	GetByID(ctx context.Context, id, userID string) (model.Tag, error)
	Update(ctx context.Context, t model.Tag) (model.Tag, error)
	SoftDelete(ctx context.Context, id, userID string) error
	UpsertForTransaction(ctx context.Context, txID, userID string, names []string) ([]model.Tag, error)
	ListForTransaction(ctx context.Context, txID string) ([]model.Tag, error)
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
	t.Name = name
	return s.repo.Update(ctx, t)
}

func (s *TagService) Delete(ctx context.Context, userID, id string) error {
	return s.repo.SoftDelete(ctx, id, userID)
}
