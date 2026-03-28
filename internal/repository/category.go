package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
)

type CategoryRepository struct {
	db *pgxpool.Pool
}

func NewCategoryRepository(db *pgxpool.Pool) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func (r *CategoryRepository) Create(ctx context.Context, c model.Category) (model.Category, error) {
	err := r.db.QueryRow(ctx, `
		INSERT INTO categories (user_id, parent_id, name, type, icon)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		c.UserID, c.ParentID, c.Name, c.Type, c.Icon,
	).Scan(&c.ID, &c.CreatedAt)
	if isUniqueViolation(err) {
		return model.Category{}, fmt.Errorf("category with this name already exists: %w", apperr.ErrConflict)
	}
	return c, err
}

func (r *CategoryRepository) GetByID(ctx context.Context, id, userID string) (model.Category, error) {
	var c model.Category
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, parent_id, name, type, icon, created_at
		FROM categories
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID,
	).Scan(&c.ID, &c.UserID, &c.ParentID, &c.Name, &c.Type, &c.Icon, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Category{}, fmt.Errorf("category %s: %w", id, apperr.ErrNotFound)
	}
	return c, err
}

func (r *CategoryRepository) ListByUser(ctx context.Context, userID string, catType model.CategoryType) ([]model.Category, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, parent_id, name, type, icon, created_at
		FROM categories
		WHERE user_id = $1 AND type = $2 AND deleted_at IS NULL
		ORDER BY name`,
		userID, catType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []model.Category
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.UserID, &c.ParentID, &c.Name, &c.Type, &c.Icon, &c.CreatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func (r *CategoryRepository) Update(ctx context.Context, c model.Category) (model.Category, error) {
	err := r.db.QueryRow(ctx, `
		UPDATE categories
		SET name = $3, icon = $4
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING id, user_id, parent_id, name, type, icon, created_at`,
		c.ID, c.UserID, c.Name, c.Icon,
	).Scan(&c.ID, &c.UserID, &c.ParentID, &c.Name, &c.Type, &c.Icon, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Category{}, fmt.Errorf("category %s: %w", c.ID, apperr.ErrNotFound)
	}
	if isUniqueViolation(err) {
		return model.Category{}, fmt.Errorf("category with this name already exists: %w", apperr.ErrConflict)
	}
	return c, err
}

// HasChildren returns true if the category has active (non-deleted) subcategories.
func (r *CategoryRepository) HasChildren(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM categories WHERE parent_id = $1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	return exists, err
}

// HasTransactions returns true if the category has any linked transactions.
func (r *CategoryRepository) HasTransactions(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM transactions WHERE category_id = $1)`, id,
	).Scan(&exists)
	return exists, err
}

func (r *CategoryRepository) SoftDelete(ctx context.Context, id, userID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE categories SET deleted_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("category %s: %w", id, apperr.ErrNotFound)
	}
	return nil
}

func (r *CategoryRepository) HardDelete(ctx context.Context, id, userID string) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM categories WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("category %s: %w", id, apperr.ErrNotFound)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
