package repository

import (
	"context"
	"fmt"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
)

// populateAccounts loads both relations in one query for the entire page.
// Soft-deleted accounts remain available to describe historical transactions.
func (r *TransactionRepository) populateAccounts(ctx context.Context, transactions []model.Transaction) ([]model.Transaction, error) {
	if len(transactions) == 0 {
		return transactions, nil
	}
	ids := make([]string, 0, len(transactions)*2)
	seen := make(map[string]bool)
	for _, tx := range transactions {
		if !seen[tx.AccountID] {
			ids = append(ids, tx.AccountID)
			seen[tx.AccountID] = true
		}
		if tx.ToAccountID != nil && !seen[*tx.ToAccountID] {
			ids = append(ids, *tx.ToAccountID)
			seen[*tx.ToAccountID] = true
		}
	}
	rows, err := r.db.Query(ctx, `
  SELECT a.id, a.owner_id, a.name, a.access_mode, a.kind, a.currency, a.icon,
         a.initial_balance, a.initial_balance_date, a.accept_transfers,
         a.deleted_at, a.created_at, a.updated_at, u.username
  FROM accounts a JOIN users u ON u.id = a.owner_id WHERE a.id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("load transaction accounts: %w", err)
	}
	defer rows.Close()
	accounts := make(map[string]model.Account, len(ids))
	for rows.Next() {
		var account model.Account
		if err := rows.Scan(&account.ID, &account.OwnerID, &account.Name, &account.AccessMode, &account.Kind,
			&account.Currency, &account.Icon, &account.InitialBalance, &account.InitialBalanceDate,
			&account.AcceptTransfers, &account.DeletedAt, &account.CreatedAt, &account.UpdatedAt, &account.OwnerUsername); err != nil {
			return nil, err
		}
		accounts[account.ID] = account
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range transactions {
		source, ok := accounts[transactions[i].AccountID]
		if !ok {
			return nil, fmt.Errorf("source account: %w", apperr.ErrNotFound)
		}
		transactions[i].Account = source
		transactions[i].AccountTo = nil
		if transactions[i].ToAccountID != nil {
			destination, ok := accounts[*transactions[i].ToAccountID]
			if !ok {
				return nil, fmt.Errorf("destination account: %w", apperr.ErrNotFound)
			}
			transactions[i].AccountTo = &destination
		}
	}
	return transactions, nil
}
