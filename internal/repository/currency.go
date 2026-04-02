package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/model"
)

type CurrencyRepository struct {
	db *pgxpool.Pool
}

func NewCurrencyRepository(db *pgxpool.Pool) *CurrencyRepository {
	return &CurrencyRepository{db: db}
}

func (r *CurrencyRepository) ListActive(ctx context.Context, extraCodes []string) ([]model.CurrencyWithRate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.code, c.name, c.symbol, c.is_active,
		       COALESCE(er.rate, 0) AS rate_to_usd
		FROM currencies c
		LEFT JOIN exchange_rates er
		       ON er.base_currency = 'USD' AND er.quote_currency = c.code
		WHERE c.is_active = true OR c.code = ANY($1)
		ORDER BY c.code`, extraCodes)
	if err != nil {
		return nil, fmt.Errorf("list currencies: %w", err)
	}
	defer rows.Close()

	var result []model.CurrencyWithRate
	for rows.Next() {
		var c model.CurrencyWithRate
		if err := rows.Scan(&c.Code, &c.Name, &c.Symbol, &c.IsActive, &c.RateToUSD); err != nil {
			return nil, err
		}
		// USD itself: rate to USD is 1
		if c.Code == "USD" {
			c.RateToUSD = 1.0
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// UpsertRates replaces all exchange rates for the given base currency.
func (r *CurrencyRepository) UpsertRates(ctx context.Context, base string, rates map[string]float64) error {
	for quote, rate := range rates {
		_, err := r.db.Exec(ctx, `
			INSERT INTO exchange_rates (base_currency, quote_currency, rate, fetched_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (base_currency, quote_currency)
			DO UPDATE SET rate = EXCLUDED.rate, fetched_at = EXCLUDED.fetched_at`,
			base, quote, rate,
		)
		if err != nil {
			return fmt.Errorf("upsert rate %s/%s: %w", base, quote, err)
		}
	}
	return nil
}

// GetRate returns the exchange rate from base to quote currency.
// Returns 0 if not found.
func (r *CurrencyRepository) GetRate(ctx context.Context, base, quote string) (float64, error) {
	if base == quote {
		return 1.0, nil
	}
	var rate float64
	err := r.db.QueryRow(ctx,
		`SELECT rate FROM exchange_rates WHERE base_currency = $1 AND quote_currency = $2`,
		base, quote,
	).Scan(&rate)
	if err != nil {
		return 0, nil //nolint:nilerr // rate not found is not an error
	}
	return rate, nil
}
