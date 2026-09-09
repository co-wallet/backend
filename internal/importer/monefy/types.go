// Package monefy reads an untrusted Monefy SQLite export without accessing the application database.
package monefy

import (
	"fmt"
	"strconv"
	"time"
)

// Amount is an exact number of thousandths of a currency unit, not ISO minor units.
// Keep this representation until persistence; converting it to float64 loses precision.
type Amount int64

func (a Amount) String() string {
	s := strconv.FormatInt(int64(a), 10)
	sign := ""
	if s[0] == '-' {
		sign, s = "-", s[1:]
	}
	for len(s) < 4 {
		s = "0" + s
	}
	return sign + s[:len(s)-3] + "." + s[len(s)-3:]
}

type Options struct {
	// SupportedCurrencies is the server's configured currency allowlist. Required.
	SupportedCurrencies []string
}

const (
	MaxFileBytes = 64 << 20
	MaxRows      = 100000 // Across all source tables, including deleted rows.
	MaxTextBytes = 64 << 10
)

type Severity string

const (
	Warning      Severity = "warning"
	Confirmation Severity = "confirmation"
	Blocking     Severity = "blocking"
)

type Diagnostic struct {
	Severity Severity
	Code     string
	Entity   string
	SourceID string
	Message  string
}

// Error denotes an unreadable or invalid export. Its partial report must not be imported.
type Error struct {
	Code, Entity, SourceID string
	Cause                  error
}

func (e *Error) Error() string {
	return fmt.Sprintf("Monefy %s (%s %s): %v", e.Code, e.Entity, e.SourceID, e.Cause)
}
func (e *Error) Unwrap() error { return e.Cause }

type Currency struct {
	ID                 int64
	Code, Name, Symbol string
	MinorUnits         int64
	IsBase             bool
}
type Account struct {
	ID, Name         string
	Icon, CurrencyID int64
	Currency         string
	CreatedAt        time.Time
	InitialBalance   Amount
	IncludedInTotal  bool
	DisabledAt       *time.Time
}
type Category struct {
	ID, Name, Type string
	Icon           int64
	DisabledAt     *time.Time
}
type Transaction struct {
	ID, AccountID, CategoryID, Type, Currency, Note string
	Amount                                          Amount // Signed: negative expense, positive income.
	CreatedAt                                       time.Time
}
type Transfer struct {
	ID, FromAccountID, ToAccountID, FromCurrency, ToCurrency, Note string
	FromAmount                                                     Amount
	ToAmount                                                       *Amount // nil when the historical amount cannot be established.
	CreatedAt                                                      time.Time
	RateID                                                         string // Empty for same-currency transfers.
}
type Rate struct {
	ID                                       string
	FromCurrencyID, ToCurrencyID, Millionths int64
	Date, CreatedAt                          time.Time
}
type Exclusion struct{ Entity, ID, Reason string }
type Report struct {
	Version      int
	Accounts     []Account
	Categories   []Category
	Transactions []Transaction
	Transfers    []Transfer
	Currencies   []Currency
	Rates        []Rate
	Exclusions   []Exclusion
	Diagnostics  []Diagnostic
	Deleted      map[string]int
}

// CanImport requires both resolution of blockers and explicit acknowledgement of exclusions.
// The API must bind any acknowledgement to the exact preview, not trust a client-side boolean.
func (r Report) CanImport() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == Blocking || d.Severity == Confirmation {
			return false
		}
	}
	return true
}
