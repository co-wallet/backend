package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/co-wallet/backend/internal/model"
)

type AccountRepository struct {
	db *pgxpool.Pool
}

func NewAccountRepository(db *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{db: db}
}

// ListByUser returns all non-deleted accounts where user is owner or member.
func (r *AccountRepository) ListByUser(ctx context.Context, userID string) ([]*model.Account, error) {
	query := `
		SELECT DISTINCT a.id, a.owner_id, a.name, a.type, a.currency, a.icon,
		       a.include_in_balance, a.initial_balance, a.initial_balance_date,
		       a.created_at, a.updated_at
		FROM accounts a
		LEFT JOIN account_members am ON am.account_id = a.id
		WHERE a.deleted_at IS NULL
		  AND (a.owner_id = $1 OR am.user_id = $1)
		ORDER BY a.created_at`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []*model.Account
	for rows.Next() {
		a := &model.Account{}
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

func (r *AccountRepository) GetByID(ctx context.Context, id string) (*model.Account, error) {
	a := &model.Account{}
	err := r.db.QueryRow(ctx, `
		SELECT id, owner_id, name, type, currency, icon,
		       include_in_balance, initial_balance, initial_balance_date,
		       deleted_at, created_at, updated_at
		FROM accounts WHERE id = $1`, id,
	).Scan(
		&a.ID, &a.OwnerID, &a.Name, &a.Type, &a.Currency, &a.Icon,
		&a.IncludeInBalance, &a.InitialBalance, &a.InitialBalanceDate,
		&a.DeletedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("account not found")
	}
	return a, err
}

func (r *AccountRepository) Create(ctx context.Context, a *model.Account) error {
	query := `
		INSERT INTO accounts (owner_id, name, type, currency, icon, include_in_balance, initial_balance, initial_balance_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`
	return r.db.QueryRow(ctx, query,
		a.OwnerID, a.Name, a.Type, a.Currency, a.Icon,
		a.IncludeInBalance, a.InitialBalance, a.InitialBalanceDate,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (r *AccountRepository) Update(ctx context.Context, a *model.Account) error {
	_, err := r.db.Exec(ctx, `
		UPDATE accounts
		SET name = $1, icon = $2, include_in_balance = $3, updated_at = now()
		WHERE id = $4 AND deleted_at IS NULL`,
		a.Name, a.Icon, a.IncludeInBalance, a.ID,
	)
	return err
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
		m := model.AccountMember{}
		if err = rows.Scan(&m.AccountID, &m.UserID, &m.Username, &m.DefaultShare); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *AccountRepository) AddMember(ctx context.Context, m model.AccountMember) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO account_members (account_id, user_id, default_share) VALUES ($1, $2, $3)
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

func (r *AccountRepository) SumShares(ctx context.Context, accountID string) (float64, error) {
	var sum float64
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(default_share), 0) FROM account_members WHERE account_id = $1`,
		accountID).Scan(&sum)
	return sum, err
}
