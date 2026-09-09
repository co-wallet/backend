package service

import (
	"fmt"
	"math"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
)

func validateTransferAmount(tx model.Transaction) error {
	if tx.AccountTo == nil {
		return fmt.Errorf("destination account is required: %w", apperr.ErrValidation)
	}
	if tx.ToAmount != nil && (*tx.ToAmount <= 0 || math.IsNaN(*tx.ToAmount) || math.IsInf(*tx.ToAmount, 0)) {
		return fmt.Errorf("destination amount must be positive: %w", apperr.ErrValidation)
	}
	if tx.AccountTo.Currency != tx.Currency && tx.ToAmount == nil {
		return fmt.Errorf("destination amount required for currency conversion: %w", apperr.ErrValidation)
	}
	if tx.AccountTo.Currency == tx.Currency && tx.ToAmount != nil && *tx.ToAmount != tx.Amount {
		return fmt.Errorf("same currency amounts must match: %w", apperr.ErrValidation)
	}
	return nil
}

func transferView(tx model.Transaction) model.Transaction {
	if !tx.ReadOnly {
		return tx
	}
	tx.Shares = nil
	tx.Tags = nil
	tx.CategoryID = nil
	tx.DefaultCurrency = nil
	tx.DefaultCurrencyAmount = nil
	tx.ExchangeRate = nil
	return tx
}
