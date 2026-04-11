package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/co-wallet/backend/internal/model"
)

//go:generate mockgen -source=currency.go -destination=mocks/mock_currency_repo.go -package=mocks

type currencyRepo interface {
	ListActive(ctx context.Context, extraCodes []string) ([]model.CurrencyWithRate, error)
	GetRate(ctx context.Context, base, quote string) (float64, error)
	UpsertRates(ctx context.Context, base string, rates map[string]float64) error
}

type CurrencyService struct {
	repo currencyRepo
}

func NewCurrencyService(repo currencyRepo) *CurrencyService {
	return &CurrencyService{repo: repo}
}

func (s *CurrencyService) ListActive(ctx context.Context, extraCodes []string) ([]model.CurrencyWithRate, error) {
	currencies, err := s.repo.ListActive(ctx, extraCodes)
	if err != nil {
		return nil, err
	}
	if currencies == nil {
		return []model.CurrencyWithRate{}, nil
	}
	return currencies, nil
}

func (s *CurrencyService) GetRate(ctx context.Context, base, quote string) (float64, error) {
	return s.repo.GetRate(ctx, base, quote)
}

// FetchAndStoreRates fetches latest rates from open.er-api.com (base: USD) and stores them.
func (s *CurrencyService) FetchAndStoreRates(ctx context.Context) error {
	type apiResponse struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Get("https://open.er-api.com/v6/latest/USD")
	if err != nil {
		return fmt.Errorf("fetch rates: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var data apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode rates response: %w", err)
	}
	if data.Result != "success" {
		return fmt.Errorf("rates API returned non-success result")
	}

	if err := s.repo.UpsertRates(ctx, "USD", data.Rates); err != nil {
		return fmt.Errorf("store rates: %w", err)
	}

	log.Printf("[currency] fetched %d exchange rates", len(data.Rates))
	return nil
}

// StartRateFetcher launches a background goroutine that fetches rates every 2 hours.
func (s *CurrencyService) StartRateFetcher(ctx context.Context) {
	go func() {
		// Fetch immediately on startup
		if err := s.FetchAndStoreRates(ctx); err != nil {
			log.Printf("[currency] initial rate fetch failed: %v", err)
		}

		ticker := time.NewTicker(2 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.FetchAndStoreRates(ctx); err != nil {
					log.Printf("[currency] rate fetch failed: %v", err)
				}
			}
		}
	}()
}
