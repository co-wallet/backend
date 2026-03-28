package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/model"
)

type AdminRepository struct {
	db *pgxpool.Pool
}

func NewAdminRepository(db *pgxpool.Pool) *AdminRepository {
	return &AdminRepository{db: db}
}

func (r *AdminRepository) ListUsers(ctx context.Context) ([]model.User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, username, email, password_hash, default_currency, is_admin, is_active, created_at, updated_at
		FROM users
		ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var result []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(
			&u.ID, &u.Username, &u.Email, &u.PasswordHash,
			&u.DefaultCurrency, &u.IsAdmin, &u.IsActive,
			&u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

func (r *AdminRepository) GetUser(ctx context.Context, id string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(ctx, `
		SELECT id, username, email, password_hash, default_currency, is_admin, is_active, created_at, updated_at
		FROM users WHERE id = $1`, id,
	).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.DefaultCurrency, &u.IsAdmin, &u.IsActive,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

func (r *AdminRepository) UpdateUser(ctx context.Context, id string, patch model.AdminUserPatch) error {
	if patch.IsActive != nil {
		if _, err := r.db.Exec(ctx,
			`UPDATE users SET is_active = $1, updated_at = now() WHERE id = $2`,
			*patch.IsActive, id); err != nil {
			return err
		}
	}
	if patch.IsAdmin != nil {
		if _, err := r.db.Exec(ctx,
			`UPDATE users SET is_admin = $1, updated_at = now() WHERE id = $2`,
			*patch.IsAdmin, id); err != nil {
			return err
		}
	}
	if patch.PasswordHash != nil {
		if _, err := r.db.Exec(ctx,
			`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
			*patch.PasswordHash, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *AdminRepository) ListAllCurrencies(ctx context.Context) ([]model.CurrencyWithRate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.code, c.name, c.symbol, c.is_active,
		       COALESCE(er.rate, 0) AS rate_to_usd
		FROM currencies c
		LEFT JOIN exchange_rates er
		       ON er.base_currency = 'USD' AND er.quote_currency = c.code
		ORDER BY c.code`)
	if err != nil {
		return nil, fmt.Errorf("list all currencies: %w", err)
	}
	defer rows.Close()

	var result []model.CurrencyWithRate
	for rows.Next() {
		var c model.CurrencyWithRate
		if err := rows.Scan(&c.Code, &c.Name, &c.Symbol, &c.IsActive, &c.RateToUSD); err != nil {
			return nil, err
		}
		if c.Code == "USD" {
			c.RateToUSD = 1.0
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (r *AdminRepository) CreateCurrency(ctx context.Context, c model.Currency) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO currencies (code, name, symbol, is_active) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (code) DO NOTHING`,
		c.Code, c.Name, c.Symbol, c.IsActive,
	)
	return err
}

func (r *AdminRepository) UpdateCurrency(ctx context.Context, code string, patch model.CurrencyPatch) error {
	if patch.Name != nil {
		if _, err := r.db.Exec(ctx,
			`UPDATE currencies SET name = $1 WHERE code = $2`, *patch.Name, code); err != nil {
			return err
		}
	}
	if patch.Symbol != nil {
		if _, err := r.db.Exec(ctx,
			`UPDATE currencies SET symbol = $1 WHERE code = $2`, *patch.Symbol, code); err != nil {
			return err
		}
	}
	if patch.IsActive != nil {
		if _, err := r.db.Exec(ctx,
			`UPDATE currencies SET is_active = $1 WHERE code = $2`, *patch.IsActive, code); err != nil {
			return err
		}
	}
	return nil
}
