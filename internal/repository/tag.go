package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/db"
	"github.com/co-wallet/backend/internal/model"
)

type TagRepository struct {
	pool *pgxpool.Pool
	db   db.DBTX
}

func NewTagRepository(pool *pgxpool.Pool) *TagRepository {
	return &TagRepository{pool: pool, db: pool}
}

func (r *TagRepository) WithTx(tx pgx.Tx) *TagRepository {
	return &TagRepository{db: tx}
}

func (r *TagRepository) ListByUser(ctx context.Context, userID string, q string) ([]model.TagWithCount, error) {
	query := `
		SELECT t.id, t.user_id, t.name, t.created_at,
		       COUNT(tt.transaction_id) AS tx_count
		FROM tags t
		LEFT JOIN transaction_tags tt ON tt.tag_id = t.id
		WHERE t.user_id = $1`
	args := []any{userID}
	if q != "" {
		query += ` AND t.name ILIKE $2`
		args = append(args, "%"+q+"%")
	}
	query += ` GROUP BY t.id ORDER BY t.name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []model.TagWithCount
	for rows.Next() {
		var tw model.TagWithCount
		if err := rows.Scan(&tw.ID, &tw.UserID, &tw.Name, &tw.CreatedAt, &tw.TxCount); err != nil {
			return nil, err
		}
		tags = append(tags, tw)
	}
	return tags, rows.Err()
}

func (r *TagRepository) GetByID(ctx context.Context, id, userID string) (model.Tag, error) {
	var t model.Tag
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, name, created_at FROM tags
		WHERE id = $1 AND user_id = $2`, id, userID,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Tag{}, fmt.Errorf("tag %s: %w", id, apperr.ErrNotFound)
	}
	return t, err
}

func (r *TagRepository) Update(ctx context.Context, t model.Tag) (model.Tag, error) {
	err := r.db.QueryRow(ctx, `
		UPDATE tags SET name = $3
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, name, created_at`, t.ID, t.UserID, t.Name,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Tag{}, fmt.Errorf("tag %s: %w", t.ID, apperr.ErrNotFound)
	}
	if isUniqueViolation(err) {
		return model.Tag{}, fmt.Errorf("tag with this name already exists: %w", apperr.ErrConflict)
	}
	return t, err
}

func (r *TagRepository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM tags WHERE id = $1 AND user_id = $2`, id, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tag %s: %w", id, apperr.ErrNotFound)
	}
	return nil
}

// UpsertForTransaction upserts tags by name for a user and links them to the transaction.
// Any tags previously linked to the transaction are replaced. Runs inside a single
// DB transaction so partial failures don't leave orphaned links.
func (r *TagRepository) UpsertForTransaction(ctx context.Context, txID, userID string, names []string) ([]model.Tag, error) {
	if r.pool == nil {
		return r.upsertForTransactionLocked(ctx, txID, userID, names)
	}
	var result []model.Tag
	err := db.WithTx(ctx, r.pool, func(pgxTx pgx.Tx) error {
		var innerErr error
		result, innerErr = r.WithTx(pgxTx).upsertForTransactionLocked(ctx, txID, userID, names)
		return innerErr
	})
	return result, err
}

func (r *TagRepository) upsertForTransactionLocked(ctx context.Context, txID, userID string, names []string) ([]model.Tag, error) {
	if _, err := r.db.Exec(ctx, `DELETE FROM transaction_tags WHERE transaction_id = $1`, txID); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}

	tags := make([]model.Tag, 0, len(names))
	for _, name := range names {
		var t model.Tag
		err := r.db.QueryRow(ctx, `
			INSERT INTO tags (user_id, name)
			VALUES ($1, $2)
			ON CONFLICT (user_id, name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id, user_id, name, created_at`, userID, name,
		).Scan(&t.ID, &t.UserID, &t.Name, &t.CreatedAt)
		if err != nil {
			return nil, err
		}
		if _, err := r.db.Exec(ctx,
			`INSERT INTO transaction_tags (transaction_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			txID, t.ID,
		); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, nil
}

// ListForTransaction returns tags linked to a transaction.
func (r *TagRepository) ListForTransaction(ctx context.Context, txID string) ([]model.Tag, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.user_id, t.name, t.created_at
		FROM tags t
		JOIN transaction_tags tt ON tt.tag_id = t.id
		WHERE tt.transaction_id = $1`, txID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// ListForTransactions returns tags linked to each transaction in one query,
// grouped by transaction ID. Used to avoid N+1 when loading a transaction list.
func (r *TagRepository) ListForTransactions(ctx context.Context, txIDs []string) (map[string][]model.Tag, error) {
	result := make(map[string][]model.Tag, len(txIDs))
	if len(txIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT tt.transaction_id, t.id, t.user_id, t.name, t.created_at
		FROM tags t
		JOIN transaction_tags tt ON tt.tag_id = t.id
		WHERE tt.transaction_id = ANY($1)`, txIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var txID string
		var t model.Tag
		if err := rows.Scan(&txID, &t.ID, &t.UserID, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		result[txID] = append(result[txID], t)
	}
	return result, rows.Err()
}
