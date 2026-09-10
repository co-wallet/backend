package repository

import (
	"context"
	"errors"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/db"
	"github.com/co-wallet/backend/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ImportRepository struct{ db db.DBTX }

func NewImportRepository(pool *pgxpool.Pool) *ImportRepository { return &ImportRepository{db: pool} }
func (r *ImportRepository) WithTx(tx pgx.Tx) *ImportRepository { return &ImportRepository{db: tx} }

// FOR UPDATE conflicts with the KEY SHARE locks taken by existing user FKs.
// Lock before reading destination state, in a separate READ COMMITTED statement so that
// ordinary writes which committed while we waited are visible to the check.
func (r *ImportRepository) LockUser(ctx context.Context, user string) error {
	var id string
	err := r.db.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, user).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.ErrUnauthorized
	}
	return err
}

// A transaction-scoped lookup holds the user's active state through commit.
func (r *ImportRepository) GetByUsername(ctx context.Context, username string) (model.User, error) {
	var u model.User
	err := r.db.QueryRow(ctx, `SELECT id,username,is_active FROM users WHERE username=$1 FOR SHARE`, username).Scan(&u.ID, &u.Username, &u.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, apperr.ErrNotFound
	}
	return u, err
}

// AccountNames includes owned and joined accounts, including archived history.
func (r *ImportRepository) AccountNames(ctx context.Context, user string, lock bool) ([]string, error) {
	if lock {
		// NOWAIT avoids a lock-order deadlock with ordinary writes waiting on the user FK.
		// Hold the lock through commit so names cannot change after validation.
		if _, err := r.db.Exec(ctx, "LOCK TABLE accounts IN SHARE ROW EXCLUSIVE MODE NOWAIT"); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
				return nil, apperr.ErrConflict
			}
			return nil, err
		}
	}
	rows, err := r.db.Query(ctx, `SELECT name FROM accounts WHERE owner_id=$1
	 OR id IN (SELECT account_id FROM account_members WHERE user_id=$1) ORDER BY id`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (r *ImportRepository) Currencies(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT code FROM currencies WHERE is_active ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var code string
		if err = rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func (r *ImportRepository) CurrencyRates(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.Query(ctx, `SELECT quote_currency,rate::text FROM exchange_rates WHERE base_currency='USD' AND rate>0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rates := map[string]string{"USD": "1"}
	for rows.Next() {
		var code, rate string
		if err := rows.Scan(&code, &rate); err != nil {
			return nil, err
		}
		rates[code] = rate
	}
	return rates, rows.Err()
}

func (r *ImportRepository) Catalog(ctx context.Context, lock bool) ([]model.Category, error) {
	q := `SELECT id, name, type, COALESCE(icon,'') FROM categories ORDER BY id`
	if lock {
		q += ` FOR SHARE`
	}
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Category{}
	for rows.Next() {
		var c model.Category
		var icon string
		if err = rows.Scan(&c.ID, &c.Name, &c.Type, &icon); err != nil {
			return nil, err
		}
		c.Icon = &icon
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *ImportRepository) Receipt(ctx context.Context, user, id string) (model.ImportResult, error) {
	var out model.ImportResult
	err := r.db.QueryRow(ctx, `SELECT preview_id,accounts,categories,reused_categories,transactions,transfers,completed_at
	 FROM monefy_imports WHERE user_id=$1 AND preview_id=$2`, user, id).Scan(&out.PreviewID, &out.Accounts, &out.Categories, &out.ReusedCategories, &out.Transactions, &out.Transfers, &out.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.ErrNotFound
	}
	return out, err
}

// Write only persists an already validated domain snapshot, on the caller's tx.
func (r *ImportRepository) Write(ctx context.Context, p model.ImportPreview) (model.ImportResult, error) {
	out := model.ImportResult{PreviewID: p.ID, Accounts: len(p.Accounts), Transactions: len(p.Report.Transactions), Transfers: len(p.Report.Transfers)}
	accounts, categories := map[string]string{}, map[string]string{}
	for i, a := range p.Report.Accounts {
		var id string
		err := r.db.QueryRow(ctx, `INSERT INTO accounts(owner_id,name,access_mode,kind,currency,icon,initial_balance,initial_balance_date)
		 VALUES($1,$2,$8,$3,$4,$5,$6::numeric,$7) RETURNING id`, p.UserID, p.Accounts[i].Name, p.Accounts[i].Kind, a.Currency, p.Accounts[i].Icon, a.InitialBalance.String(), a.CreatedAt, p.Accounts[i].AccessMode).Scan(&id)
		if err != nil {
			return out, err
		}
		accounts[a.ID] = id
		for _, m := range p.Accounts[i].Members {
			if _, err = r.db.Exec(ctx, `INSERT INTO account_members(account_id,user_id,default_share) VALUES($1,$2,$3)`, id, m.UserID, m.DefaultShare); err != nil {
				return out, err
			}
		}
	}
	for _, c := range p.Categories {
		id := c.ExistingID
		if id == "" {
			err := r.db.QueryRow(ctx, `INSERT INTO categories(user_id,name,type,icon) VALUES($1,$2,$3,$4)
			 ON CONFLICT DO NOTHING RETURNING id`, p.UserID, c.Name, c.Type, c.Icon).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return out, apperr.ErrConflict
			}
			if err != nil {
				return out, err
			}
			out.Categories++
		} else {
			out.ReusedCategories++
		}
		categories[c.SourceID] = id
	}
	for _, t := range p.Report.Transactions {
		amount := t.Amount
		if amount < 0 {
			amount = -amount
		}
		var id string
		var baseAmount *string
		if t.DefaultCurrencyAmount != nil {
			value := t.DefaultCurrencyAmount.String()
			baseAmount = &value
		}
		err := r.db.QueryRow(ctx, `INSERT INTO transactions(account_id,type,amount,currency,category_id,description,date,created_by,default_currency,default_currency_amount)
		 VALUES($1,$2,$3::numeric,$4,$5,$6,$7,$8,NULLIF($9,''),$10::numeric) RETURNING id`,
			accounts[t.AccountID], t.Type, amount.String(), t.Currency, categories[t.CategoryID], t.Note, t.CreatedAt, p.UserID, t.DefaultCurrency, baseAmount).Scan(&id)
		if err != nil {
			return out, err
		}
		if err = r.writeImportShares(ctx, id, p.TransactionShares[t.ID]); err != nil {
			return out, err
		}
	}
	for _, t := range p.Report.Transfers {
		var id string
		err := r.db.QueryRow(ctx, `INSERT INTO transactions(account_id,to_account_id,type,amount,to_amount,currency,description,date,created_by)
		 VALUES($1,$2,'transfer',$3::numeric,$4::numeric,$5,$6,$7,$8) RETURNING id`,
			accounts[t.FromAccountID], accounts[t.ToAccountID], t.FromAmount.String(), t.ToAmount.String(), t.FromCurrency, t.Note, t.CreatedAt, p.UserID).Scan(&id)
		if err != nil {
			return out, err
		}
		if err = r.writeImportShares(ctx, id, p.TransferShares[t.ID]); err != nil {
			return out, err
		}
	}
	err := r.db.QueryRow(ctx, `INSERT INTO monefy_imports(preview_id,user_id,accounts,categories,reused_categories,transactions,transfers)
	 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING completed_at`, p.ID, p.UserID, out.Accounts, out.Categories, out.ReusedCategories, out.Transactions, out.Transfers).Scan(&out.CompletedAt)
	return out, err
}

func (r *ImportRepository) writeImportShares(ctx context.Context, id string, shares []model.ImportShare) error {
	for _, share := range shares {
		if _, err := r.db.Exec(ctx, `INSERT INTO transaction_shares(transaction_id,user_id,amount) VALUES($1,$2,$3::numeric)`, id, share.UserID, share.Amount); err != nil {
			return err
		}
	}
	return nil
}
