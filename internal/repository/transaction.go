package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
)

type TransactionRepository struct {
	db *pgxpool.Pool
}

func NewTransactionRepository(db *pgxpool.Pool) *TransactionRepository {
	return &TransactionRepository{db: db}
}

func (r *TransactionRepository) Create(ctx context.Context, tx model.Transaction) (model.Transaction, error) {
	err := r.db.QueryRow(ctx, `
		INSERT INTO transactions
		    (account_id, to_account_id, type, amount, currency, exchange_rate,
		     default_currency, default_currency_amount,
		     category_id, description, date, include_in_balance, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, created_at, updated_at`,
		tx.AccountID, tx.ToAccountID, tx.Type, tx.Amount, tx.Currency, tx.ExchangeRate,
		tx.DefaultCurrency, tx.DefaultCurrencyAmount,
		tx.CategoryID, tx.Description, tx.Date, tx.IncludeInBalance, tx.CreatedBy,
	).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt)
	if err != nil {
		return model.Transaction{}, err
	}

	if err = r.upsertShares(ctx, tx.ID, tx.Shares); err != nil {
		return model.Transaction{}, fmt.Errorf("upsert shares: %w", err)
	}
	return tx, nil
}

func (r *TransactionRepository) GetByID(ctx context.Context, id string) (model.Transaction, error) {
	var tx model.Transaction
	err := r.db.QueryRow(ctx, `
		SELECT id, account_id, to_account_id, type, amount, currency, exchange_rate,
		       default_currency, default_currency_amount,
		       category_id, description, date, include_in_balance, created_by, created_at, updated_at
		FROM transactions WHERE id = $1`, id,
	).Scan(
		&tx.ID, &tx.AccountID, &tx.ToAccountID, &tx.Type, &tx.Amount, &tx.Currency, &tx.ExchangeRate,
		&tx.DefaultCurrency, &tx.DefaultCurrencyAmount,
		&tx.CategoryID, &tx.Description, &tx.Date, &tx.IncludeInBalance, &tx.CreatedBy,
		&tx.CreatedAt, &tx.UpdatedAt,
	)
	if isNoRows(err) {
		return model.Transaction{}, fmt.Errorf("transaction %s: %w", id, apperr.ErrNotFound)
	}
	if err != nil {
		return model.Transaction{}, err
	}
	tx.Shares, err = r.listShares(ctx, id)
	return tx, err
}

func (r *TransactionRepository) List(ctx context.Context, userID string, f model.TransactionFilter) ([]model.Transaction, error) {
	// Build a query that returns transactions where the user is a member of the account
	// or is the creator, filtered by the provided criteria.
	args := []any{userID}
	q := `
		SELECT DISTINCT t.id, t.account_id, t.to_account_id, t.type, t.amount, t.currency,
		       t.exchange_rate, t.default_currency, t.default_currency_amount,
		       t.category_id, t.description, t.date, t.include_in_balance,
		       t.created_by, t.created_at, t.updated_at
		FROM transactions t
		JOIN accounts a ON a.id = t.account_id
		WHERE (a.owner_id = $1 OR EXISTS (
		    SELECT 1 FROM account_members am WHERE am.account_id = t.account_id AND am.user_id = $1
		))`

	n := 2
	if len(f.AccountIDs) > 0 {
		q += fmt.Sprintf(" AND t.account_id = ANY($%d)", n)
		args = append(args, f.AccountIDs)
		n++
	}
	if len(f.CategoryIDs) > 0 {
		q += fmt.Sprintf(" AND t.category_id = ANY($%d)", n)
		args = append(args, f.CategoryIDs)
		n++
	}
	if f.DateFrom != nil {
		q += fmt.Sprintf(" AND t.date >= $%d", n)
		args = append(args, *f.DateFrom)
		n++
	}
	if f.DateTo != nil {
		q += fmt.Sprintf(" AND t.date <= $%d", n)
		args = append(args, *f.DateTo)
		n++
	}
	if len(f.TagIDs) > 0 {
		if f.TagMode == "and" {
			// Transaction must have ALL specified tags
			for _, tagID := range f.TagIDs {
				q += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM transaction_tags tt WHERE tt.transaction_id = t.id AND tt.tag_id = $%d)`, n)
				args = append(args, tagID)
				n++
			}
		} else {
			// OR mode: transaction must have at least one of the tags
			q += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM transaction_tags tt WHERE tt.transaction_id = t.id AND tt.tag_id = ANY($%d))`, n)
			args = append(args, f.TagIDs)
			n++
		}
	}

	q += " ORDER BY t.date DESC, t.created_at DESC"

	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset := (f.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	q += fmt.Sprintf(" LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []model.Transaction
	for rows.Next() {
		var tx model.Transaction
		if err := rows.Scan(
			&tx.ID, &tx.AccountID, &tx.ToAccountID, &tx.Type, &tx.Amount, &tx.Currency,
			&tx.ExchangeRate, &tx.DefaultCurrency, &tx.DefaultCurrencyAmount,
			&tx.CategoryID, &tx.Description, &tx.Date, &tx.IncludeInBalance,
			&tx.CreatedBy, &tx.CreatedAt, &tx.UpdatedAt,
		); err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load shares per transaction
	for i := range txs {
		txs[i].Shares, err = r.listShares(ctx, txs[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return txs, nil
}

func (r *TransactionRepository) Update(ctx context.Context, tx model.Transaction) (model.Transaction, error) {
	err := r.db.QueryRow(ctx, `
		UPDATE transactions
		SET amount = $2, category_id = $3, description = $4,
		    date = $5, include_in_balance = $6, default_currency_amount = $7, updated_at = now()
		WHERE id = $1
		RETURNING id, account_id, to_account_id, type, amount, currency, exchange_rate,
		          default_currency, default_currency_amount,
		          category_id, description, date, include_in_balance, created_by, created_at, updated_at`,
		tx.ID, tx.Amount, tx.CategoryID, tx.Description, tx.Date, tx.IncludeInBalance, tx.DefaultCurrencyAmount,
	).Scan(
		&tx.ID, &tx.AccountID, &tx.ToAccountID, &tx.Type, &tx.Amount, &tx.Currency, &tx.ExchangeRate,
		&tx.DefaultCurrency, &tx.DefaultCurrencyAmount,
		&tx.CategoryID, &tx.Description, &tx.Date, &tx.IncludeInBalance, &tx.CreatedBy,
		&tx.CreatedAt, &tx.UpdatedAt,
	)
	if isNoRows(err) {
		return model.Transaction{}, fmt.Errorf("transaction %s: %w", tx.ID, apperr.ErrNotFound)
	}
	if err != nil {
		return model.Transaction{}, err
	}

	if tx.Shares != nil {
		if err = r.upsertShares(ctx, tx.ID, tx.Shares); err != nil {
			return model.Transaction{}, fmt.Errorf("upsert shares: %w", err)
		}
	}
	tx.Shares, err = r.listShares(ctx, tx.ID)
	return tx, err
}

func (r *TransactionRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM transactions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("transaction %s: %w", id, apperr.ErrNotFound)
	}
	return nil
}

func (r *TransactionRepository) GetMemberDefaults(ctx context.Context, accountID string) ([]model.AccountMember, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, default_share FROM account_members WHERE account_id = $1`, accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []model.AccountMember
	for rows.Next() {
		var m model.AccountMember
		if err := rows.Scan(&m.UserID, &m.DefaultShare); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// --- internal helpers ---

func (r *TransactionRepository) upsertShares(ctx context.Context, txID string, shares []model.TransactionShare) error {
	_, err := r.db.Exec(ctx, `DELETE FROM transaction_shares WHERE transaction_id = $1`, txID)
	if err != nil {
		return err
	}
	for _, s := range shares {
		_, err = r.db.Exec(ctx, `
			INSERT INTO transaction_shares (transaction_id, user_id, amount, is_custom)
			VALUES ($1, $2, $3, $4)`,
			txID, s.UserID, s.Amount, s.IsCustom,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *TransactionRepository) listShares(ctx context.Context, txID string) ([]model.TransactionShare, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, transaction_id, user_id, amount, is_custom
		FROM transaction_shares WHERE transaction_id = $1`, txID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var shares []model.TransactionShare
	for rows.Next() {
		var s model.TransactionShare
		if err := rows.Scan(&s.ID, &s.TransactionID, &s.UserID, &s.Amount, &s.IsCustom); err != nil {
			return nil, err
		}
		shares = append(shares, s)
	}
	return shares, rows.Err()
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
