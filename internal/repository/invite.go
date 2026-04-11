package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/db"
	"github.com/co-wallet/backend/internal/model"
)

type InviteRepository struct {
	db db.DBTX
}

func NewInviteRepository(pool *pgxpool.Pool) *InviteRepository {
	return &InviteRepository{db: pool}
}

// WithTx returns a copy of the repository scoped to the given transaction.
func (r *InviteRepository) WithTx(tx pgx.Tx) *InviteRepository {
	return &InviteRepository{db: tx}
}

func (r *InviteRepository) Create(ctx context.Context, inv model.Invite) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO invites (email, token, created_by, expires_at)
		VALUES ($1, $2, $3, $4)`,
		inv.Email, inv.Token, inv.CreatedBy, inv.ExpiresAt,
	)
	return err
}

func (r *InviteRepository) GetByToken(ctx context.Context, token string) (*model.Invite, error) {
	inv := &model.Invite{}
	err := r.db.QueryRow(ctx, `
		SELECT id, email, token, created_by, used_at, expires_at, created_at
		FROM invites WHERE token = $1`, token,
	).Scan(&inv.ID, &inv.Email, &inv.Token, &inv.CreatedBy, &inv.UsedAt, &inv.ExpiresAt, &inv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("invite not found")
	}
	return inv, err
}

func (r *InviteRepository) MarkUsed(ctx context.Context, token string) error {
	now := time.Now()
	_, err := r.db.Exec(ctx, `UPDATE invites SET used_at = $1 WHERE token = $2`, now, token)
	return err
}

func (r *InviteRepository) ListByCreator(ctx context.Context, createdBy string) ([]model.Invite, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, email, token, created_by, used_at, expires_at, created_at
		FROM invites WHERE created_by = $1 ORDER BY created_at DESC`, createdBy)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()

	var result []model.Invite
	for rows.Next() {
		var inv model.Invite
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Token, &inv.CreatedBy,
			&inv.UsedAt, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, inv)
	}
	return result, rows.Err()
}

// ListAll is used by admin to see all invites.
func (r *InviteRepository) ListAll(ctx context.Context) ([]model.Invite, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, email, token, created_by, used_at, expires_at, created_at
		FROM invites ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list all invites: %w", err)
	}
	defer rows.Close()

	var result []model.Invite
	for rows.Next() {
		var inv model.Invite
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Token, &inv.CreatedBy,
			&inv.UsedAt, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, inv)
	}
	return result, rows.Err()
}
