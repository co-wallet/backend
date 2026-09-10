package service

import (
	"context"
	"errors"
	"math"
	"math/big"
	"sort"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/model"
)

// All operation shares use NUMERIC(15,4) units. Largest remainders receive
// the remaining units; UUID order breaks ties deterministically, including after retries.
func importShares(amount monefy.Amount, members []model.ImportMember) []model.ImportShare {
	total := int64(amount) * 10
	if total < 0 {
		total = -total
	}
	units := make([]int64, len(members))
	remainders := make([]int64, len(members))
	order := make([]int, len(members))
	var distributed int64
	for i, m := range members {
		product := new(big.Int).Mul(big.NewInt(total), big.NewInt(int64(math.Round(m.DefaultShare*10000))))
		q, r := new(big.Int), new(big.Int)
		q.QuoRem(product, big.NewInt(10000), r)
		units[i], remainders[i], order[i] = q.Int64(), r.Int64(), i
		distributed += units[i]
	}
	sort.SliceStable(order, func(i, j int) bool { return remainders[order[i]] > remainders[order[j]] })
	for i := int64(0); i < total-distributed; i++ {
		units[order[i]]++
	}
	out := make([]model.ImportShare, len(members))
	for i, m := range members {
		out[i] = model.ImportShare{UserID: m.UserID, Amount: new(big.Rat).SetFrac(big.NewInt(units[i]), big.NewInt(10000)).FloatString(4)}
	}
	return out
}

func importProportion(amount monefy.Amount, share float64) *big.Rat {
	return new(big.Rat).Mul(new(big.Rat).SetFrac(big.NewInt(int64(amount)), big.NewInt(1000)), new(big.Rat).SetFrac(big.NewInt(int64(math.Round(share*10000))), big.NewInt(10000)))
}

func prepareImportShares(ctx context.Context, repo importRepo, p *model.ImportPreview) error {
	p.TransactionShares = map[string][]model.ImportShare{}
	p.TransferShares = map[string][]model.ImportShare{}
	accounts := map[string]*model.ImportAccount{}
	balances := map[string]map[string]*big.Rat{}
	for i := range p.Accounts {
		a := &p.Accounts[i]
		config, exists := p.AccountAccess[a.SourceID]
		if !exists {
			config = model.ImportAccountAccess{AccessMode: model.AccountAccessModePersonal}
		}
		members, err := creationMembers(ctx, repo, p.UserID, model.CreateAccountReq{AccessMode: config.AccessMode, Members: config.Members})
		if err != nil {
			if !errors.Is(err, apperr.ErrValidation) {
				return err
			}
			p.Report.Diagnostics = append(p.Report.Diagnostics, monefy.Diagnostic{Severity: monefy.Blocking, Code: "target_account_members", Entity: "Account", SourceID: a.SourceID, Message: "Укажите активных уникальных участников, включая владельца. Доли — от 0 до 1, до четырёх знаков, сумма ровно 1; личный счёт — только владелец."})
			a.AccessMode = config.AccessMode
			continue
		}
		sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })
		a.AccessMode = config.AccessMode
		accounts[a.SourceID] = a
		balances[a.SourceID] = map[string]*big.Rat{}
		for _, m := range members {
			initial := importProportion(p.Report.Accounts[i].InitialBalance, m.DefaultShare)
			a.Members = append(a.Members, model.ImportMember{UserID: m.UserID, Username: m.Username, DefaultShare: m.DefaultShare, InitialBalance: initial.FloatString(8)})
			balances[a.SourceID][m.UserID] = initial
		}
	}
	for _, d := range p.Report.Diagnostics {
		if d.Severity == monefy.Blocking {
			return nil
		}
	}
	apply := func(account string, shares []model.ImportShare, subtract bool) {
		for _, share := range shares {
			value, _ := new(big.Rat).SetString(share.Amount)
			if subtract {
				balances[account][share.UserID].Sub(balances[account][share.UserID], value)
			} else {
				balances[account][share.UserID].Add(balances[account][share.UserID], value)
			}
		}
	}
	for _, t := range p.Report.Transactions {
		if a := accounts[t.AccountID]; a != nil {
			shares := importShares(t.Amount, a.Members)
			p.TransactionShares[t.ID] = shares
			apply(t.AccountID, shares, t.Type == "expense")
		}
	}
	for _, t := range p.Report.Transfers {
		if a := accounts[t.FromAccountID]; a != nil {
			shares := importShares(t.FromAmount, a.Members)
			p.TransferShares[t.ID] = shares
			apply(t.FromAccountID, shares, true)
		}
		if a := accounts[t.ToAccountID]; a != nil && t.ToAmount != nil {
			for _, m := range a.Members {
				balances[t.ToAccountID][m.UserID].Add(balances[t.ToAccountID][m.UserID], importProportion(*t.ToAmount, m.DefaultShare))
			}
		}
	}
	for _, a := range accounts {
		for i := range a.Members {
			a.Members[i].FinalBalance = balances[a.SourceID][a.Members[i].UserID].FloatString(8)
		}
	}
	return nil
}

// Re-resolve every shared participant inside the import transaction. A deleted,
// deactivated or renamed/replaced user invalidates the snapshot before any deletion.
func validateImportMembers(ctx context.Context, repo importRepo, p model.ImportPreview) error {
	for _, a := range p.Accounts {
		if !a.AccessMode.IsValid() || len(a.Members) == 0 {
			return importError("preview_stale", apperr.ErrConflict)
		}
		if a.AccessMode != model.AccountAccessModeShared {
			continue
		}
		for _, m := range a.Members {
			user, err := repo.GetByUsername(ctx, m.Username)
			if err != nil && !errors.Is(err, apperr.ErrNotFound) {
				return err
			}
			if err != nil || !user.IsActive || user.ID != m.UserID {
				return importError("members_changed", apperr.ErrConflict)
			}
		}
	}
	return nil
}
