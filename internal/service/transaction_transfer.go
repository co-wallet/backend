package service

import (
	"fmt"
	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/model"
	"math"
)

func validateTransferAmount(tx model.Transaction) error {
	if tx.ToAmount != nil && (*tx.ToAmount <= 0 || math.IsNaN(*tx.ToAmount) || math.IsInf(*tx.ToAmount, 0)) {
		return fmt.Errorf("destination amount must be positive: %w", apperr.ErrValidation)
	}
	if tx.ToCurrency != tx.Currency && tx.ToAmount == nil {
		return fmt.Errorf("destination amount required for currency conversion: %w", apperr.ErrValidation)
	}
	if tx.ToCurrency == tx.Currency && tx.ToAmount != nil && *tx.ToAmount != tx.Amount {
		return fmt.Errorf("same currency amounts must match: %w", apperr.ErrValidation)
	}
	return nil
}

func scaleShares(shares []model.TransactionShare, amount float64) []model.TransactionShare {
	shares = append([]model.TransactionShare(nil), shares...)
	total := 0.0
	for _, share := range shares {
		total += share.Amount
	}
	distributed := 0.0
	for i := range shares {
		part := 0.0
		if total > 0 {
			part = math.Round(shares[i].Amount/total*amount*10000) / 10000
		}
		if i == len(shares)-1 {
			part = math.Round((amount-distributed)*10000) / 10000
		}
		shares[i].Amount = part
		distributed += part
	}
	return shares
}

func transferView(tx model.Transaction, userID string) model.Transaction {
	if !tx.ReadOnly {
		return tx
	}
	amount := 0.0
	for _, share := range tx.ToShares {
		if share.UserID == userID {
			amount = share.Amount
		}
	}
	tx.RecipientAmount = &amount
	tx.Shares = nil
	tx.Tags = nil
	tx.CategoryID = nil
	tx.DefaultCurrency = nil
	tx.DefaultCurrencyAmount = nil
	tx.ExchangeRate = nil
	return tx
}
