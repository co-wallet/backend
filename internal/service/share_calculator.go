package service

import (
	"fmt"
	"math"

	"github.com/co-wallet/backend/internal/model"
)

// calculateShares distributes the total amount among account members according
// to their default_share ratios. The last member absorbs any rounding remainder
// so that the sum of shares always equals totalAmount exactly.
func calculateShares(totalAmount float64, members []model.AccountMember) ([]model.TransactionShare, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("account has no members")
	}

	shares := make([]model.TransactionShare, len(members))
	var distributed float64

	for i, m := range members {
		var amount float64
		if i < len(members)-1 {
			amount = roundCents(totalAmount * m.DefaultShare)
			distributed += amount
		} else {
			// last member absorbs remainder to avoid floating-point drift
			amount = roundCents(totalAmount - distributed)
		}
		shares[i] = model.TransactionShare{
			UserID:   m.UserID,
			Amount:   amount,
			IsCustom: false,
		}
	}
	return shares, nil
}

// validateCustomShares checks that custom share amounts sum to the transaction total.
func validateCustomShares(totalAmount float64, reqs []model.ShareReq) error {
	var sum float64
	for _, s := range reqs {
		sum += s.Amount
	}
	if math.Abs(sum-totalAmount) > 0.01 {
		return fmt.Errorf("sum of shares (%.2f) must equal transaction amount (%.2f)", sum, totalAmount)
	}
	return nil
}

func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
}
