package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

func (r *CategoryRepository) Create(ctx context.Context, userID string, req model.CreateCategoryReq) (model.Category, error) {
	c := model.Category{
		UserID:   userID,
		ParentID: req.ParentID,
		Name:     req.Name,
		Type:     req.Type,
		Icon:     req.Icon,
	}
	err := r.db.QueryRow(ctx, `
		INSERT INTO categories (user_id, parent_id, name, type, icon)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		c.UserID, c.ParentID, c.Name, c.Type, c.Icon,
	).Scan(&c.ID, &c.CreatedAt)
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
	if err == pgx.ErrNoRows {
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

func (r *CategoryRepository) Update(ctx context.Context, id, userID string, req model.UpdateCategoryReq) (model.Category, error) {
	var c model.Category
	err := r.db.QueryRow(ctx, `
		UPDATE categories
		SET name      = COALESCE($3, name),
		    icon      = COALESCE($4, icon)
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING id, user_id, parent_id, name, type, icon, created_at`,
		id, userID, req.Name, req.Icon,
	).Scan(&c.ID, &c.UserID, &c.ParentID, &c.Name, &c.Type, &c.Icon, &c.CreatedAt)
	if err == pgx.ErrNoRows {
		return model.Category{}, fmt.Errorf("category %s: %w", id, apperr.ErrNotFound)
	}
	return c, err
}

// HasTransactions returns true if the category (or any of its descendants) has linked transactions.
// NOTE: always returns false until Phase 4 adds the transactions table.
func (r *CategoryRepository) HasTransactions(_ context.Context, _ string) (bool, error) {
	return false, nil
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
