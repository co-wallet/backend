package monefy_test

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"math/big"
	"os"
	"testing"

	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/stretchr/testify/require"
)

// TestLocalReference is opt-in research verification, not CSV import support.
// Paths are supplied locally; financial exports must never enter the repository.
func TestLocalReference(t *testing.T) {
	dbPath, csvPath := os.Getenv("MONEFY_REFERENCE_DB"), os.Getenv("MONEFY_REFERENCE_CSV")
	if dbPath == "" || csvPath == "" {
		t.Skip("local reference exports not configured")
	}
	before, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	f, err := os.Open(dbPath)
	require.NoError(t, err)
	r, err := monefy.Parse(context.Background(), f, monefy.Options{SupportedCurrencies: []string{"RUB", "TRY", "EUR", "USD", "LKR", "ILS"}})
	require.NoError(t, err)
	require.NoError(t, f.Close())
	after, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(before), sha256.Sum256(after), "source file changed")
	for _, d := range r.Diagnostics {
		require.NotEqual(t, monefy.Blocking, d.Severity, "blocking diagnostic: %s", d.Code)
	}
	accounts, categories := map[string]monefy.Account{}, map[string]monefy.Category{}
	for _, a := range r.Accounts {
		accounts[a.ID] = a
	}
	for _, c := range r.Categories {
		categories[c.ID] = c
	}
	type key struct{ date, account, category, amount, currency, note string }
	expected, actual := map[key]int{}, map[key]int{}
	for _, op := range r.Transactions {
		actual[key{op.CreatedAt.Format("02.01.2006"), accounts[op.AccountID].Name, categories[op.CategoryID].Name, op.Amount.String(), op.Currency, op.Note}]++
	}
	fxCount, balances := 0, 0
	for _, op := range r.Transfers {
		require.NotNil(t, op.ToAmount)
		actual[key{op.CreatedAt.Format("02.01.2006"), accounts[op.FromAccountID].Name, "ExpenseTransfer", (-op.FromAmount).String(), op.FromCurrency, op.Note}]++
		actual[key{op.CreatedAt.Format("02.01.2006"), accounts[op.ToAccountID].Name, "IncomeTransfer", op.ToAmount.String(), op.ToCurrency, op.Note}]++
		if op.FromCurrency != op.ToCurrency {
			fxCount++
		}
	}
	for _, a := range r.Accounts {
		if a.InitialBalance != 0 {
			actual[key{a.CreatedAt.Format("02.01.2006"), a.Name, "InitialBalance", a.InitialBalance.String(), a.Currency, ""}]++
			balances++
		}
	}
	csvFile, err := os.Open(csvPath)
	require.NoError(t, err)
	rows, err := csv.NewReader(csvFile).ReadAll()
	require.NoError(t, err)
	require.NoError(t, csvFile.Close())
	for _, row := range rows[1:] {
		require.Len(t, row, 8)
		amount, ok := new(big.Rat).SetString(row[3])
		require.True(t, ok)
		amount.Mul(amount, big.NewRat(1000, 1))
		require.True(t, amount.IsInt())
		require.True(t, amount.Num().IsInt64())
		expected[key{row[0], row[1], row[2], monefy.Amount(amount.Num().Int64()).String(), row[4], row[7]}]++
	}
	// Report only counts on mismatch, never personal descriptions or balances.
	mismatches := 0
	for k, count := range expected {
		if actual[k] != count {
			mismatches++
		}
	}
	for k := range actual {
		if expected[k] == 0 {
			mismatches++
		}
	}
	require.Zero(t, mismatches, "normalized DB/CSV multiset differs")
	t.Logf("verified %d transactions, %d transfers (%d FX), %d balances, %d exclusions; source SHA-256 unchanged", len(r.Transactions), len(r.Transfers), fxCount, balances, len(r.Exclusions))
}
