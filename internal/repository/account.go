package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
)

type AccountRepository struct {
	db *pgxpool.Pool
}

func NewAccountRepository(db *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{db: db}
}

// ListByUser returns all non-deleted accounts where user is owner or member.
func (r *AccountRepository) ListByUser(ctx context.Context, userID string) ([]model.Account, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT a.id, a.owner_id, a.name, a.type, a.currency, a.icon,
		       a.include_in_balance, a.initial_balance, a.initial_balance_date,
		       a.created_at, a.updated_at
		FROM accounts a
		LEFT JOIN account_members am ON am.account_id = a.id
		WHERE a.deleted_at IS NULL
		  AND (a.owner_id = $1 OR am.user_id = $1)
		ORDER BY a.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []model.Account
	for rows.Next() {
		var a model.Account
		if err = rows.Scan(
			&a.ID, &a.OwnerID, &a.Name, &a.Type, &a.Currency, &a.Icon,
			&a.IncludeInBalance, &a.InitialBalance, &a.InitialBalanceDate,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// GetByID returns a non-deleted account by ID. Returns apperr.ErrNotFound if absent or deleted.
func (r *AccountRepository) GetByID(ctx context.Context, id string) (model.Account, error) {
	var a model.Account
	err := r.db.QueryRow(ctx, `
		SELECT id, owner_id, name, type, currency, icon,
		       include_in_balance, initial_balance, initial_balance_date,
		       created_at, updated_at
		FROM accounts WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(
		&a.ID, &a.OwnerID, &a.Name, &a.Type, &a.Currency, &a.Icon,
		&a.IncludeInBalance, &a.InitialBalance, &a.InitialBalanceDate,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return model.Account{}, fmt.Errorf("account %s: %w", id, apperr.ErrNotFound)
	}
	return a, err
}

// Create inserts a new account and returns it with DB-generated fields populated.
func (r *AccountRepository) Create(ctx context.Context, a model.Account) (model.Account, error) {
	err := r.db.QueryRow(ctx, `
		INSERT INTO accounts (owner_id, name, type, currency, icon, include_in_balance, initial_balance, initial_balance_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`,
		a.OwnerID, a.Name, a.Type, a.Currency, a.Icon,
		a.IncludeInBalance, a.InitialBalance, a.InitialBalanceDate,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// Update persists account field changes. Returns the updated account.
func (r *AccountRepository) Update(ctx context.Context, a model.Account) (model.Account, error) {
	err := r.db.QueryRow(ctx, `
		UPDATE accounts
		SET name = $1, icon = $2, include_in_balance = $3,
		    initial_balance = $4, initial_balance_date = $5,
		    updated_at = now()
		WHERE id = $6 AND deleted_at IS NULL
		RETURNING updated_at`,
		a.Name, a.Icon, a.IncludeInBalance, a.InitialBalance, a.InitialBalanceDate, a.ID,
	).Scan(&a.UpdatedAt)
	return a, err
}

func (r *AccountRepository) SoftDelete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE accounts SET deleted_at = now(), updated_at = now() WHERE id = $1`, id)
	return err
}

// Members

func (r *AccountRepository) GetMembers(ctx context.Context, accountID string) ([]model.AccountMember, error) {
	rows, err := r.db.Query(ctx, `
		SELECT am.account_id, am.user_id, u.username, am.default_share
		FROM account_members am
		JOIN users u ON u.id = am.user_id
		WHERE am.account_id = $1
		ORDER BY u.username`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []model.AccountMember
	for rows.Next() {
		var m model.AccountMember
		if err = rows.Scan(&m.AccountID, &m.UserID, &m.Username, &m.DefaultShare); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *AccountRepository) AddMember(ctx context.Context, m model.AccountMember) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO account_members (account_id, user_id, default_share) VALUES ($1, $2, $3)
		ON CONFLICT (account_id, user_id) DO UPDATE SET default_share = EXCLUDED.default_share`,
		m.AccountID, m.UserID, m.DefaultShare)
	return err
}

func (r *AccountRepository) UpdateMemberShare(ctx context.Context, accountID, userID string, share float64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE account_members SET default_share = $1 WHERE account_id = $2 AND user_id = $3`,
		share, accountID, userID)
	return err
}

func (r *AccountRepository) RemoveMember(ctx context.Context, accountID, userID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM account_members WHERE account_id = $1 AND user_id = $2`,
		accountID, userID)
	return err
}

// ListBalancesByUser returns per-account balance breakdown for all accounts the user
// owns or belongs to. Amounts are converted to displayCurrency using USD-pivot rates.
func (r *AccountRepository) ListBalancesByUser(ctx context.Context, userID, displayCurrency string) (map[string]model.AccountBalance, error) {
	q := fmt.Sprintf(`
		WITH per_account AS (
		    SELECT
		        a.id,
		        a.currency,
		        a.initial_balance
		            * COALESCE((SELECT am_me.default_share FROM account_members am_me
		                        WHERE am_me.account_id = a.id AND am_me.user_id = $1), 1.0)
		            + COALESCE(SUM(CASE WHEN t.type = 'income'  AND t.include_in_balance THEN ts.amount ELSE 0 END), 0)
		            - COALESCE(SUM(CASE WHEN t.type = 'expense' AND t.include_in_balance THEN ts.amount ELSE 0 END), 0)
		        AS balance_native,
		        a.initial_balance
		            + COALESCE(SUM(CASE WHEN t.type = 'income'  AND t.include_in_balance THEN t.amount ELSE 0 END), 0)
		            - COALESCE(SUM(CASE WHEN t.type = 'expense' AND t.include_in_balance THEN t.amount ELSE 0 END), 0)
		        AS total_native
		    FROM accounts a
		    LEFT JOIN transactions t ON t.account_id = a.id
		        AND (a.initial_balance_date IS NULL OR t.date >= a.initial_balance_date)
		    LEFT JOIN transaction_shares ts ON ts.transaction_id = t.id AND ts.user_id = $1
		    WHERE a.deleted_at IS NULL
		      AND (a.owner_id = $1 OR EXISTS (
		               SELECT 1 FROM account_members am
		               WHERE am.account_id = a.id AND am.user_id = $1))
		    GROUP BY a.id, a.currency, a.initial_balance
		)
		SELECT
		    id,
		    balance_native,
		    %s AS balance_display,
		    total_native,
		    %s AS total_display
		FROM per_account`,
		convertExpr("balance_native", "currency", 2),
		convertExpr("total_native", "currency", 2),
	)

	rows, err := r.db.Query(ctx, q, userID, displayCurrency)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]model.AccountBalance)
	for rows.Next() {
		var b model.AccountBalance
		if err = rows.Scan(&b.AccountID, &b.BalanceNative, &b.BalanceDisplay, &b.TotalNative, &b.TotalDisplay); err != nil {
			return nil, err
		}
		result[b.AccountID] = b
	}
	return result, rows.Err()
}

func (r *AccountRepository) IsMember(ctx context.Context, accountID, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM account_members WHERE account_id = $1 AND user_id = $2
			UNION
			SELECT 1 FROM accounts WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		)`, accountID, userID,
	).Scan(&exists)
	return exists, err
}
