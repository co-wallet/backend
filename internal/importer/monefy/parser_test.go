package monefy_test

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/v11.sql
var fixtureSQL string

var options = monefy.Options{SupportedCurrencies: []string{"RUB", "TRY"}}

func fixture(t *testing.T, change string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(fixtureSQL)
	require.NoError(t, err)
	if change != "" {
		_, err = db.Exec(change)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func parse(t *testing.T, change string) monefy.Report {
	t.Helper()
	r, err := monefy.Parse(context.Background(), bytes.NewReader(fixture(t, change)), options)
	require.NoError(t, err)
	return r
}

func TestParse(t *testing.T) {
	data := fixture(t, "")
	original := bytes.Clone(data)
	r, err := monefy.Parse(context.Background(), bytes.NewReader(data), options)
	require.NoError(t, err)
	require.Equal(t, original, data)
	require.True(t, r.CanImport())
	require.Equal(t, 11, r.Version)
	require.Len(t, r.Accounts, 3)
	require.Equal(t, "cash", r.Accounts[0].ID)
	require.Equal(t, "-1234.567", r.Accounts[0].InitialBalance.String())
	require.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 100, time.UTC), r.Accounts[0].CreatedAt)
	require.Equal(t, "9007199254740.993", r.Accounts[2].InitialBalance.String())
	require.NotNil(t, r.Accounts[2].DisabledAt)
	require.False(t, r.Accounts[2].IncludedInTotal)
	require.Len(t, r.Categories, 2)
	require.Equal(t, "expense", r.Categories[0].Type)
	require.Equal(t, "income", r.Categories[1].Type)
	require.Len(t, r.Transactions, 3)
	require.Equal(t, "expense-1", r.Transactions[0].ID)
	require.Equal(t, "expense-2", r.Transactions[1].ID)
	require.Equal(t, monefy.Amount(-12345), r.Transactions[0].Amount)
	require.Equal(t, r.Transactions[0].Amount, r.Transactions[1].Amount)
	require.Equal(t, time.Date(2024, 1, 2, 0, 0, 0, 700, time.UTC), r.Transactions[0].CreatedAt)
	require.Equal(t, monefy.Amount(999999), r.Transactions[2].Amount)
	require.Equal(t, "TRY", r.Transactions[2].Currency)
	require.Len(t, r.Transfers, 2)
	require.Equal(t, monefy.Amount(1234567), r.Transfers[0].FromAmount)
	require.Equal(t, "411.521", r.Transfers[0].ToAmount.String())
	require.Equal(t, "revision-2", r.Transfers[0].RateID)
	require.Equal(t, "1234.567", r.Transfers[1].ToAmount.String())
	require.Equal(t, map[string]int{"Account": 1, "Category": 1, "Transaction": 1, "Transfer": 1, "CurrencyRate": 1}, r.Deleted)
	require.Len(t, r.Rates, 4)
	require.Len(t, r.Currencies, 2)
	require.Equal(t, "disabled_account", r.Diagnostics[0].Code)
}

func TestDiagnostics(t *testing.T) {
	tests := []struct {
		name, sql, code string
		severity        monefy.Severity
	}{
		{"deleted account", `UPDATE "Transaction" SET AccountId='deleted' WHERE Id='expense-1'`, "deleted_reference", monefy.Confirmation},
		{"deleted category", `UPDATE "Transaction" SET CategoryId='old' WHERE Id='expense-1'`, "deleted_reference", monefy.Confirmation},
		{"deleted transfer account", `UPDATE Transfer SET AccountToId='deleted' WHERE Id='fx'`, "deleted_reference", monefy.Confirmation},
		{"missing account", `UPDATE "Transaction" SET AccountId='missing' WHERE Id='expense-1'`, "invalid_reference", monefy.Blocking},
		{"missing category", `UPDATE "Transaction" SET CategoryId='missing' WHERE Id='expense-1'`, "invalid_reference", monefy.Blocking},
		{"missing transfer account", `UPDATE Transfer SET AccountFromId='missing' WHERE Id='fx'`, "invalid_reference", monefy.Blocking},
		{"unknown currency", `UPDATE Account SET CurrencyId=123 WHERE Id='cash'`, "unknown_currency", monefy.Blocking},
		{"unconfigured currency", `UPDATE Currency SET AlphabeticCode='EUR' WHERE Id=949`, "unknown_currency", monefy.Blocking},
		{"invalid rate reference", `UPDATE CurrencyRate SET CurrencyToId=123 WHERE Id='earlier'`, "invalid_reference", monefy.Blocking},
		{"no historical rate", `DELETE FROM CurrencyRate WHERE Id != 'future'`, "ambiguous_rate", monefy.Blocking},
		{"inverse only", `UPDATE CurrencyRate SET CurrencyFromId=949,CurrencyToId=643`, "ambiguous_rate", monefy.Blocking},
		{"conflicting revisions", `UPDATE CurrencyRate SET CreatedOn=638399232000000000 WHERE Id='revision-1'`, "ambiguous_rate", monefy.Blocking},
		{"overflow", `UPDATE Transfer SET AmountCents=9223372036854775807 WHERE Id='fx'; UPDATE CurrencyRate SET RateCents=9223372036854775807`, "amount_range", monefy.Blocking},
		{"truncated to zero", `UPDATE Transfer SET AmountCents=1 WHERE Id='fx'`, "amount_range", monefy.Blocking},
		{"schedule", `INSERT INTO Schedule(Id) VALUES ('schedule')`, "unsupported_schedule", monefy.Blocking},
		{"transaction schedule", `UPDATE "Transaction" SET ScheduleId='schedule' WHERE Id='income'`, "unsupported_schedule", monefy.Blocking},
		{"setting", `INSERT INTO Setting VALUES ('config', 'value')`, "unsupported_setting", monefy.Blocking},
		{"extra table", `CREATE TABLE Extra (Id TEXT)`, "unsupported_schema", monefy.Blocking},
		{"extra column", `ALTER TABLE Account ADD COLUMN Unsupported TEXT`, "unsupported_column", monefy.Blocking},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := parse(t, tt.sql)
			require.False(t, r.CanImport())
			var found bool
			for _, d := range r.Diagnostics {
				if d.Code == tt.code {
					require.Equal(t, tt.severity, d.Severity)
					found = true
				}
			}
			require.True(t, found, "%+v", r.Diagnostics)
			if tt.severity == monefy.Confirmation {
				require.Len(t, r.Exclusions, 1)
				require.NotEmpty(t, r.Exclusions[0].ID)
			}
			if tt.code == "ambiguous_rate" || tt.code == "amount_range" {
				require.Nil(t, r.Transfers[0].ToAmount)
			}
		})
	}
}

func TestInvalidData(t *testing.T) {
	tests := []struct{ name, sql, code string }{
		{"version", "PRAGMA user_version=12", "version"},
		{"missing table", "DROP TABLE Category", "schema"},
		{"view", "ALTER TABLE Category RENAME TO Old; CREATE VIEW Category AS SELECT * FROM Old", "schema"},
		{"missing column", "ALTER TABLE Account RENAME COLUMN CurrencyId TO Currency", "schema"},
		{"negative ticks", "UPDATE Account SET CreatedOn=-1", "data"},
		{"ticks overflow", "UPDATE Account SET CreatedOn=3155378976000000000", "data"},
		{"null ticks", "UPDATE Account SET CreatedOn=NULL", "data"},
		{"invalid disabled", "UPDATE Account SET DisabledOn=-1", "data"},
		{"invalid deleted", "UPDATE Account SET DeletedOn=-1", "data"},
		{"fractional amount", `UPDATE "Transaction" SET AmountCents=1.5`, "data"},
		{"blob", `UPDATE "Transaction" SET Note=x'1234'`, "data"},
		{"invalid UTF8", `UPDATE "Transaction" SET Note=CAST(x'80' AS TEXT)`, "data"},
		{"negative amount", `UPDATE "Transaction" SET AmountCents=-1`, "data"},
		{"negative transfer", `UPDATE Transfer SET AmountCents=-1`, "data"},
		{"zero transfer", `UPDATE Transfer SET AmountCents=0`, "data"},
		{"self transfer", `UPDATE Transfer SET AccountToId=AccountFromId`, "data"},
		{"bad type", "UPDATE Category SET CategoryType=2", "data"},
		{"bad flag", "UPDATE Account SET IsIncludedInTotalBalance=2", "data"},
		{"empty name", "UPDATE Account SET Name=' '", "data"},
		{"empty id", "UPDATE Account SET Id='' WHERE Id='cash'", "data"},
		{"duplicate id", "ALTER TABLE Category RENAME TO Original; CREATE TABLE Category AS SELECT * FROM Original; INSERT INTO Category SELECT * FROM Original; DROP TABLE Original", "data"},
		{"bad currency", "UPDATE Currency SET AlphabeticCode='???'", "data"},
		{"bad currency number", "UPDATE Currency SET NumericCode=1", "data"},
		{"bad minor units", "UPDATE Currency SET MinorUnits=-1", "data"},
		{"zero rate", "UPDATE CurrencyRate SET RateCents=0", "data"},
		{"text limit", `UPDATE "Transaction" SET Note=replace(hex(zeroblob(40000)), '00', 'xx')`, "data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := monefy.Parse(context.Background(), bytes.NewReader(fixture(t, tt.sql)), options)
			var parseErr *monefy.Error
			require.ErrorAs(t, err, &parseErr)
			require.Equal(t, tt.code, parseErr.Code)
			require.False(t, report.CanImport())
		})
	}
}

func TestHistoricalRateSelection(t *testing.T) {
	tests := []struct{ name, sql, rate, amount string }{
		{"latest revision", "", "revision-2", "411.521"},
		{"exact date", "UPDATE Transfer SET CreatedOn=638397504000000000", "revision-2", "411.521"},
		{"earlier date", "UPDATE Transfer SET CreatedOn=638396640000000000", "earlier", "123.456"},
		{"future revision becomes applicable", "UPDATE Transfer SET CreatedOn=638399232000000000", "future", "617.283"},
		{"same value same timestamp", "UPDATE CurrencyRate SET CreatedOn=638399232000000000,RateCents=333333 WHERE Id='revision-1'", "revision-1", "411.521"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := parse(t, tt.sql)
			require.True(t, r.CanImport())
			require.Equal(t, tt.rate, r.Transfers[0].RateID)
			require.Equal(t, tt.amount, r.Transfers[0].ToAmount.String())
		})
	}
}

func TestAmountString(t *testing.T) {
	for value, expected := range map[monefy.Amount]string{0: "0.000", 1: "0.001", -1: "-0.001", 1000: "1.000", -9223372036854775808: "-9223372036854775.808", 9223372036854775807: "9223372036854775.807"} {
		require.Equal(t, expected, value.String())
	}
}

func TestInputErrors(t *testing.T) {
	t.Run("WAL header", func(t *testing.T) {
		data := fixture(t, "")
		data[18], data[19] = 2, 2
		r, err := monefy.Parse(context.Background(), bytes.NewReader(data), options)
		var parseErr *monefy.Error
		require.ErrorAs(t, err, &parseErr)
		require.Equal(t, "format", parseErr.Code)
		require.False(t, r.CanImport())
	})
	t.Run("corrupt", func(t *testing.T) {
		for _, data := range [][]byte{nil, []byte("not sqlite"), fixture(t, "")[:200]} {
			_, err := monefy.Parse(context.Background(), bytes.NewReader(data), options)
			require.Error(t, err)
		}
	})
	t.Run("read error", func(t *testing.T) {
		_, err := monefy.Parse(context.Background(), brokenReader{}, options)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := monefy.Parse(ctx, strings.NewReader(""), options)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("missing allowlist", func(t *testing.T) {
		_, err := monefy.Parse(context.Background(), strings.NewReader(""), monefy.Options{})
		var parseErr *monefy.Error
		require.ErrorAs(t, err, &parseErr)
		require.Equal(t, "configuration", parseErr.Code)
	})
	t.Run("file limit", func(t *testing.T) {
		_, err := monefy.Parse(context.Background(), io.LimitReader(zeroReader{}, monefy.MaxFileBytes+1), options)
		var parseErr *monefy.Error
		require.ErrorAs(t, err, &parseErr)
		require.Equal(t, "limit", parseErr.Code)
	})
	t.Run("row limit includes deleted", func(t *testing.T) {
		data := fixture(t, `WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<100001) INSERT INTO Setting SELECT 'setting-'||x, '' FROM n`)
		_, err := monefy.Parse(context.Background(), bytes.NewReader(data), options)
		var parseErr *monefy.Error
		require.ErrorAs(t, err, &parseErr)
		require.Equal(t, "limit", parseErr.Code)
	})
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestTemporaryFilesRemoved(t *testing.T) {
	data := fixture(t, "")
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	_, err := monefy.Parse(context.Background(), brokenReader{}, options)
	require.True(t, errors.Is(err, io.ErrUnexpectedEOF))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	_, err = monefy.Parse(context.Background(), bytes.NewReader(data), options)
	require.NoError(t, err)
	entries, err = os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestSourceMetadataAndDateExtremes(t *testing.T) {
	r := parse(t, `ALTER TABLE Account ADD COLUMN LocalHashCode INTEGER;
ALTER TABLE Account ADD COLUMN RemoteHashCode INTEGER;
UPDATE Account SET CreatedOn=0 WHERE Id='cash';
UPDATE Account SET CreatedOn=3155378975999999999 WHERE Id='travel';
UPDATE Category SET DisabledOn=638397504000000000 WHERE Id='food';
INSERT INTO Schedule(Id,DeletedOn) VALUES ('deleted',638397504000000000);`)
	require.True(t, r.CanImport())
	require.Equal(t, time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), r.Accounts[0].CreatedAt)
	require.Equal(t, time.Date(9999, 12, 31, 23, 59, 59, 999999900, time.UTC), r.Accounts[2].CreatedAt)
	require.NotNil(t, r.Categories[0].DisabledAt)
	require.Equal(t, 1, r.Deleted["Schedule"])
}
