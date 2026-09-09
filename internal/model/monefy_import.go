package model

import (
	"time"

	"github.com/co-wallet/backend/internal/importer/monefy"
)

// ImportPreview is a private, immutable snapshot stored outside the application DB.
// Financial amounts stay exact until SQL persistence and HTTP serialization.
type ImportPreview struct {
	Mode                 ImportMode
	Replacement          *ImportReplacement
	ID, UserID, SHA256   string
	ExpiresAt            time.Time
	Report               monefy.Report
	Accounts             []ImportAccount
	Categories           []ImportCategory
	CategoryIcons        map[string]string
	AccountIcons         map[string]string
	PeriodFrom, PeriodTo *time.Time
}

type ImportMode string

const (
	ImportEmpty   ImportMode = "empty"
	ImportReplace ImportMode = "replace"
)

// ImportReplacement stores a deletion manifest, never a backup of the old history.
type ImportReplacement struct {
	Fingerprint string
	Accounts    []ImportReplacementAccount
	Counts      map[string]int
	Blockers    map[string]int
}

type ImportReplacementAccount struct {
	ID, Name, Currency string
	DeletedAt          *time.Time
}

type ImportAccount struct {
	SourceID string
	Kind     AccountKind
	Icon     string
	Balance  string
}

type ImportCategory struct {
	SourceID, ExistingID, Name, Type, Icon string
}

type ImportResult struct {
	PreviewID                                                       string
	Accounts, Categories, ReusedCategories, Transactions, Transfers int
	CompletedAt                                                     time.Time
}

type ImportAvailability struct{ Reasons []string }
