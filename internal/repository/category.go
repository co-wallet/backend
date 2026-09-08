package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

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
		INSERT INTO categories (user_id, name, type, icon)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		c.UserID, c.Name, c.Type, c.Icon,
	).Scan(&c.ID, &c.CreatedAt)
	if isUniqueViolation(err) {
		return model.Category{}, fmt.Errorf("category with this name already exists: %w", apperr.ErrConflict)
	}
	return c, err
}

func (r *CategoryRepository) GetByID(ctx context.Context, id, userID string) (model.Category, error) {
	var c model.Category
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, name, type, icon, created_at,
        EXISTS (SELECT 1 FROM hidden_categories h WHERE h.category_id = categories.id AND h.user_id = $2)
		FROM categories
		WHERE id = $1`,
		id, userID,
	).Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Icon, &c.CreatedAt, &c.Hidden)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Category{}, fmt.Errorf("category %s: %w", id, apperr.ErrNotFound)
	}
	return c, err
}

func (r *CategoryRepository) ListByUser(ctx context.Context, userID string, catType model.CategoryType) ([]model.Category, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, name, type, icon, created_at,
        EXISTS (SELECT 1 FROM hidden_categories h WHERE h.category_id = categories.id AND h.user_id = $1)
		FROM categories
		WHERE type = $2
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
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Icon, &c.CreatedAt, &c.Hidden); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func (r *CategoryRepository) Update(ctx context.Context, c model.Category) (model.Category, error) {
	err := r.db.QueryRow(ctx, `
		UPDATE categories
		SET name = $2, icon = $3
		WHERE id = $1
		RETURNING id, user_id, name, type, icon, created_at`,
		c.ID, c.Name, c.Icon,
	).Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Icon, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Category{}, fmt.Errorf("category %s: %w", c.ID, apperr.ErrNotFound)
	}
	if isUniqueViolation(err) {
		return model.Category{}, fmt.Errorf("category with this name already exists: %w", apperr.ErrConflict)
	}
	return c, err
}

// HasTransactions returns true if the category has any linked transactions.
func (r *CategoryRepository) HasTransactions(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM transactions WHERE category_id = $1)`, id,
	).Scan(&exists)
	return exists, err
}

func (r *CategoryRepository) HardDelete(ctx context.Context, id, userID string) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM categories WHERE id = $1`,
		id,
	)
	if isForeignKeyViolation(err) {
		return fmt.Errorf("category has linked transactions: %w", apperr.ErrConflict)
	}
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

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func (r *CategoryRepository) SetHidden(ctx context.Context, id, userID string, hidden bool) error {
	if hidden {
		_, err := r.db.Exec(ctx, `INSERT INTO hidden_categories (category_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, userID)
		if isForeignKeyViolation(err) {
			return apperr.ErrNotFound
		}
		return err
	}
	_, err := r.db.Exec(ctx, `DELETE FROM hidden_categories WHERE category_id = $1 AND user_id = $2`, id, userID)
	return err
}
