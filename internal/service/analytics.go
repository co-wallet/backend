package service

import (
	"context"

	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
)

type AnalyticsService struct {
	repo *repository.AnalyticsRepository
}

func NewAnalyticsService(repo *repository.AnalyticsRepository) *AnalyticsService {
	return &AnalyticsService{repo: repo}
}

func (s *AnalyticsService) Summary(ctx context.Context, f model.AnalyticsFilter) (model.AnalyticsSummary, error) {
	return s.repo.Summary(ctx, f)
}

func (s *AnalyticsService) ByCategory(ctx context.Context, f model.AnalyticsFilter) ([]model.CategoryStat, error) {
	stats, err := s.repo.ByCategory(ctx, f)
	if err != nil {
		return nil, err
	}
	if stats == nil {
		return []model.CategoryStat{}, nil
	}
	return stats, nil
}

func (s *AnalyticsService) ByTag(ctx context.Context, f model.AnalyticsFilter) ([]model.TagStat, error) {
	stats, err := s.repo.ByTag(ctx, f)
	if err != nil {
		return nil, err
	}
	if stats == nil {
		return []model.TagStat{}, nil
	}
	return stats, nil
}
