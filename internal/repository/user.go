package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/db"
	"github.com/co-wallet/backend/internal/model"
)

type UserRepository struct {
	db db.DBTX
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: pool}
}

// WithTx returns a copy of the repository scoped to the given transaction.
func (r *UserRepository) WithTx(tx pgx.Tx) *UserRepository {
	return &UserRepository{db: tx}
}

func (r *UserRepository) Create(ctx context.Context, u *model.User) error {
	query := `
		INSERT INTO users (username, email, password_hash, default_currency, is_admin, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`
	return r.db.QueryRow(ctx, query,
		u.Username, u.Email, u.PasswordHash,
		u.DefaultCurrency, u.IsAdmin, u.IsActive,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	return r.scanOne(ctx,
		`SELECT id, username, email, password_hash, default_currency, is_admin, is_active, created_at, updated_at
		 FROM users WHERE id = $1 AND is_active = true`, id)
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return r.scanOne(ctx,
		`SELECT id, username, email, password_hash, default_currency, is_admin, is_active, created_at, updated_at
		 FROM users WHERE email = $1`, email)
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	return r.scanOne(ctx,
		`SELECT id, username, email, password_hash, default_currency, is_admin, is_active, created_at, updated_at
		 FROM users WHERE username = $1`, username)
}

// ListActive returns id, username, email for all active users — used for member picker.
func (r *UserRepository) ListActive(ctx context.Context) ([]model.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, username, email, password_hash, default_currency, is_admin, is_active, created_at, updated_at
		 FROM users WHERE is_active = true ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var result []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash,
			&u.DefaultCurrency, &u.IsAdmin, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (r *UserRepository) UpdateCurrency(ctx context.Context, id, currency string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET default_currency = $1, updated_at = now() WHERE id = $2`,
		currency, id)
	return err
}

func (r *UserRepository) scanOne(ctx context.Context, query string, args ...any) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash,
		&u.DefaultCurrency, &u.IsAdmin, &u.IsActive,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("user not found")
	}
	return u, err
}
