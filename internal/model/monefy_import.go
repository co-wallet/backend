package model

import (
	"time"

	"github.com/co-wallet/backend/internal/importer/monefy"
)

// ImportPreview is a private, immutable snapshot stored outside the application DB.
// Financial amounts stay exact until SQL persistence and HTTP serialization.
type ImportPreview struct {
	ID, UserID, SHA256   string
	ExpiresAt            time.Time
	Report               monefy.Report
	Accounts             []ImportAccount
	Categories           []ImportCategory
	PeriodFrom, PeriodTo *time.Time
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
