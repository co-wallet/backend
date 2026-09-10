package service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/importer/preview"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/service/mocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ImportSuite struct {
	suite.Suite
	repo     *mocks.MockimportRepo
	store    *mocks.MockimportStore
	svc      *ImportService
	user, id string
}

func TestImportSuite(t *testing.T) { suite.Run(t, new(ImportSuite)) }
func (s *ImportSuite) SetupTest() {
	c := gomock.NewController(s.T())
	s.repo = mocks.NewMockimportRepo(c)
	s.store = mocks.NewMockimportStore(c)
	s.svc = &ImportService{repo: s.repo, store: s.store, withTx: func(ctx context.Context, fn func(importRepo) error) error { return fn(s.repo) }}
	s.user = uuid.NewString()
	s.id = uuid.NewString()
}
func (s *ImportSuite) TestAccessAndNotEmptyBeforeParsing() {
	_, err := s.svc.Preview(context.Background(), "", strings.NewReader(""), model.ImportEmpty)
	s.ErrorIs(err, apperr.ErrUnauthorized)
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{Reasons: []string{"owned_accounts"}}, nil)
	_, err = s.svc.Preview(context.Background(), s.user, strings.NewReader(""), model.ImportEmpty)
	s.ErrorContains(err, "account_not_empty")
}
func (s *ImportSuite) TestMalformedFile() {
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Currencies(gomock.Any()).Return([]string{"RUB"}, nil)
	_, err := s.svc.Preview(context.Background(), s.user, strings.NewReader("date,amount\n2024-01-01,12"), model.ImportEmpty)
	s.ErrorContains(err, "source_format")
}
func (s *ImportSuite) TestPreviewExactBalancesAndOptions() {
	path := filepath.Join(s.T().TempDir(), "source.db")
	d, err := sql.Open("sqlite", path)
	s.Require().NoError(err)
	fixture, err := os.ReadFile("../importer/monefy/testdata/v11.sql")
	s.Require().NoError(err)
	_, err = d.Exec(string(fixture))
	s.Require().NoError(err)
	_, err = d.Exec(`UPDATE Account SET InitialBalanceCents=1000 WHERE Id='travel'`)
	s.Require().NoError(err)
	s.Require().NoError(d.Close())
	f, err := os.Open(path)
	s.Require().NoError(err)
	defer f.Close() //nolint:errcheck
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Currencies(gomock.Any()).Return([]string{"RUB", "TRY"}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), false).Return([]model.Category{{ID: "shared-food", Name: " FOOD ", Type: model.CategoryTypeExpense}}, nil)
	s.store.EXPECT().Save(gomock.Any()).Return(nil)
	p, err := s.svc.Preview(context.Background(), s.user, f, model.ImportEmpty)
	s.Require().NoError(err)
	s.Equal("-3728.391", p.Accounts[0].Balance)
	s.True(strings.HasPrefix(p.Accounts[0].Icon, "preset:cash|"))
	byID := map[string]string{}
	for _, a := range p.Accounts {
		byID[a.SourceID] = a.Balance
	}
	s.Equal("1412.520", byID["travel"])
	s.Equal("shared-food", p.Categories[0].ExistingID)
	s.True(p.Report.CanImport())
	for _, account := range p.Accounts {
		s.Equal(model.AccountKindSpending, account.Kind)
	}
	s.Len(p.SHA256, 64)
	s.store.EXPECT().Load(s.user, p.ID).Return(p, nil)
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), false).Return([]model.Category{{ID: "shared-food", Name: " FOOD ", Type: model.CategoryTypeExpense}}, nil)
	s.store.EXPECT().Save(gomock.Any()).Return(nil)
	configured, err := s.svc.Configure(context.Background(), s.user, p.ID, map[string]model.AccountKind{"cash": "spending", "travel": "deposit", "reserve": "spending"}, nil, nil, nil)
	s.Require().NoError(err)
	s.NotEqual(p.ID, configured.ID)
	s.Equal(p.SHA256, configured.SHA256)
	s.True(configured.Report.CanImport())
	s.Equal(p.CategoryIcons, configured.CategoryIcons)
	s.Equal(p.AccountIcons, configured.AccountIcons)
	s.Equal(p.Report.Transactions, configured.Report.Transactions)
	s.Equal(p.Report.Transfers, configured.Report.Transfers)
	for _, account := range configured.Accounts {
		want := model.AccountKindSpending
		if account.SourceID == "travel" {
			want = model.AccountKindDeposit
		}
		s.Equal(want, account.Kind)
	}
	for _, snapshot := range []model.ImportPreview{p, configured} {
		s.store.EXPECT().Load(s.user, snapshot.ID).Return(snapshot, nil)
		s.repo.EXPECT().LockUser(gomock.Any(), s.user).Return(nil)
		s.repo.EXPECT().Receipt(gomock.Any(), s.user, snapshot.ID).Return(model.ImportResult{}, apperr.ErrNotFound)
		s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
		s.repo.EXPECT().Catalog(gomock.Any(), true).Return([]model.Category{{ID: "shared-food", Name: " FOOD ", Type: model.CategoryTypeExpense}}, nil)
		s.repo.EXPECT().Currencies(gomock.Any()).Return([]string{"RUB", "TRY"}, nil)
		s.repo.EXPECT().Write(gomock.Any(), snapshot).Return(model.ImportResult{PreviewID: snapshot.ID}, nil)
		s.store.EXPECT().Delete(s.user, snapshot.ID).Return(nil)
		result, confirmErr := s.svc.Confirm(context.Background(), s.user, snapshot.ID, true, false)
		s.Require().NoError(confirmErr)
		s.Equal(snapshot.ID, result.PreviewID)
	}
}
func (s *ImportSuite) TestInvalidKinds() {
	p := model.ImportPreview{Report: monefy.Report{Accounts: []monefy.Account{{ID: "a"}}}}
	s.store.EXPECT().Load(s.user, s.id).Return(p, nil)
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	_, err := s.svc.Configure(context.Background(), s.user, s.id, map[string]model.AccountKind{"a": "unknown"}, nil, nil, nil)
	s.ErrorIs(err, apperr.ErrValidation)
}
func (s *ImportSuite) confirmStart(p model.ImportPreview) {
	s.repo.EXPECT().LockUser(gomock.Any(), s.user).Return(nil)
	s.repo.EXPECT().Receipt(gomock.Any(), s.user, s.id).Return(model.ImportResult{}, apperr.ErrNotFound)
	s.store.EXPECT().Load(s.user, s.id).Return(p, nil)
}
func (s *ImportSuite) TestConfirmBlocker() {
	s.confirmStart(model.ImportPreview{Report: monefy.Report{Diagnostics: []monefy.Diagnostic{{Severity: monefy.Blocking}}}})
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, true, false)
	s.ErrorContains(err, "preview_blocked")
}
func (s *ImportSuite) TestConfirmRequiresAcknowledgement() {
	s.confirmStart(model.ImportPreview{Report: monefy.Report{Exclusions: []monefy.Exclusion{{ID: "removed"}}}})
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, false, false)
	s.ErrorContains(err, "exclusions_not_confirmed")
}
func (s *ImportSuite) TestConfirmOtherUser() {
	s.repo.EXPECT().LockUser(gomock.Any(), s.user).Return(nil)
	s.repo.EXPECT().Receipt(gomock.Any(), s.user, s.id).Return(model.ImportResult{}, apperr.ErrNotFound)
	s.store.EXPECT().Load(s.user, s.id).Return(model.ImportPreview{}, apperr.ErrNotFound)
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, true, false)
	s.ErrorIs(err, apperr.ErrNotFound)
}
func (s *ImportSuite) TestRepeatUsesReceiptWithoutFile() {
	want := model.ImportResult{PreviewID: s.id, CompletedAt: time.Now()}
	s.repo.EXPECT().LockUser(gomock.Any(), s.user).Return(nil)
	s.repo.EXPECT().Receipt(gomock.Any(), s.user, s.id).Return(want, nil)
	s.store.EXPECT().Delete(s.user, s.id).Return(nil)
	got, err := s.svc.Confirm(context.Background(), s.user, s.id, false, false)
	s.NoError(err)
	s.Equal(want, got)
}
func (s *ImportSuite) TestCatalogChanged() {
	s.confirmStart(model.ImportPreview{Categories: []model.ImportCategory{{SourceID: "c", Name: "Food", Type: "expense"}}, Report: monefy.Report{Categories: []monefy.Category{{ID: "c", Name: "Food", Type: "expense"}}}})
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), true).Return([]model.Category{{ID: "new", Name: "Food", Type: "expense"}}, nil)
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, true, false)
	s.ErrorContains(err, "catalog_changed")
}

func (s *ImportSuite) TestDestinationDiagnostics() {
	for _, tt := range []struct {
		name, code string
		change     func(*model.ImportPreview)
	}{
		{"zero", "target_amount_range", func(p *model.ImportPreview) { p.Report.Transactions[0].Amount = 0 }},
		{"overflow", "target_amount_range", func(p *model.ImportPreview) { p.Report.Accounts[0].InitialBalance = 100000000000000 }},
		{"duplicate", "target_ambiguous_category", func(p *model.ImportPreview) {
			p.Report.Categories = append(p.Report.Categories, monefy.Category{ID: "c2", Name: " FOOD ", Type: "expense"})
		}},
		{"long name", "target_name_length", func(p *model.ImportPreview) { p.Report.Accounts[0].Name = strings.Repeat("я", 101) }},
		{"before balance", "target_before_initial_balance", func(p *model.ImportPreview) {
			p.Report.Transactions[0].CreatedAt = p.Report.Accounts[0].CreatedAt.Add(-48 * time.Hour)
		}},
	} {
		s.Run(tt.name, func() {
			p := model.ImportPreview{ID: s.id, UserID: s.user, Report: monefy.Report{
				Accounts:     []monefy.Account{{ID: "a", Name: "Cash", Currency: "RUB", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}},
				Categories:   []monefy.Category{{ID: "c", Name: "Food", Type: "expense"}},
				Transactions: []monefy.Transaction{{ID: "t", AccountID: "a", CategoryID: "c", Type: "expense", Amount: -12345, CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}},
			}}
			tt.change(&p)
			s.store.EXPECT().Load(s.user, s.id).Return(p, nil)
			s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
			s.repo.EXPECT().Catalog(gomock.Any(), false).Return(nil, nil)
			s.store.EXPECT().Save(gomock.Any()).Return(nil)
			got, err := s.svc.Configure(context.Background(), s.user, s.id, map[string]model.AccountKind{"a": "spending"}, nil, nil, nil)
			s.Require().NoError(err)
			found := false
			for _, d := range got.Report.Diagnostics {
				if d.Code == tt.code && d.Severity == monefy.Blocking {
					found = true
				}
			}
			s.True(found)
		})
	}
}

func (s *ImportSuite) TestCategoryIconOptionsPersistAcrossConfiguration() {
	p := model.ImportPreview{ID: s.id, Report: monefy.Report{Categories: []monefy.Category{{ID: "new", Name: "New", Type: "expense"}, {ID: "shared", Name: "Shared", Type: "income"}}}, Categories: []model.ImportCategory{{SourceID: "new"}, {SourceID: "shared", ExistingID: "existing"}}}
	sharedIcon := "preset:work|green|none"
	catalog := []model.Category{{ID: "existing", Name: "Shared", Type: model.CategoryTypeIncome, Icon: &sharedIcon}}
	s.store.EXPECT().Load(s.user, s.id).Return(p, nil)
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), false).Return(catalog, nil)
	s.store.EXPECT().Save(gomock.Any()).DoAndReturn(func(saved model.ImportPreview) error {
		s.Equal("preset:groceries|red|none", saved.Categories[0].Icon)
		s.Equal(sharedIcon, saved.Categories[1].Icon)
		return nil
	})
	configured, err := s.svc.Configure(context.Background(), s.user, s.id, nil, map[string]string{"new": "preset:groceries|red|none"}, nil, nil)
	s.Require().NoError(err)
	s.Empty(p.CategoryIcons)
	s.NotEqual(p.ID, configured.ID)
	s.store.EXPECT().Load(s.user, configured.ID).Return(configured, nil)
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), false).Return(catalog, nil)
	s.store.EXPECT().Save(gomock.Any()).Return(nil)
	again, err := s.svc.Configure(context.Background(), s.user, configured.ID, nil, nil, nil, nil)
	s.Require().NoError(err)
	s.Equal(configured.Categories, again.Categories)
}

func (s *ImportSuite) TestInvalidCategoryIcons() {
	for _, tt := range []struct{ name, id, icon string }{
		{"emoji", "new", "📁"}, {"custom text", "new", "custom:AB"},
		{"unknown preset", "new", "preset:unsupported"},
		{"extra fields", "new", "preset:other|blue|blue|red"},
		{"bad color", "new", "preset:other|invisible|none"},
		{"unknown category", "missing", "preset:other"}, {"shared category", "shared", "preset:other"},
	} {
		s.Run(tt.name, func() {
			p := model.ImportPreview{Categories: []model.ImportCategory{{SourceID: "new"}, {SourceID: "shared", ExistingID: "existing"}}}
			s.store.EXPECT().Load(s.user, s.id).Return(p, nil)
			s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
			_, err := s.svc.Configure(context.Background(), s.user, s.id, nil, map[string]string{tt.id: tt.icon}, nil, nil)
			s.ErrorIs(err, apperr.ErrValidation)
			var typed *ImportError
			s.Require().ErrorAs(err, &typed)
			s.Equal("invalid_category_icons", typed.Code)
		})
	}
}

func (s *ImportSuite) TestPreviewStorageCapacityError() {
	p := model.ImportPreview{}
	s.store.EXPECT().Load(s.user, s.id).Return(p, nil)
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), false).Return(nil, nil)
	s.store.EXPECT().Save(gomock.Any()).Return(preview.ErrCapacity)
	_, err := s.svc.Configure(context.Background(), s.user, s.id, nil, nil, nil, nil)
	s.ErrorIs(err, apperr.ErrConflict)
	s.ErrorIs(err, preview.ErrCapacity)
	var typed *ImportError
	s.Require().ErrorAs(err, &typed)
	s.Equal("preview_storage_full", typed.Code)
}

func (s *ImportSuite) TestSemanticAppearanceOnFirstPreview() {
	cases := []struct{ name, kind, preset string }{
		{"Продукты", "expense", "groceries"}, {"ПРОДУКТЫ", "expense", "groceries"},
		{"Супермаркет", "expense", "groceries"}, {"Groceries", "expense", "groceries"},
		{"Кафе", "expense", "cafe"}, {"КОФЕ", "expense", "cafe"}, {"Coffee shop", "expense", "cafe"},
		{"Транспорт", "expense", "bus"}, {"ПРОЕЗД", "expense", "bus"}, {"Transport", "expense", "bus"},
		{"Зарплата", "income", "salary"}, {"ЗАРАБОТНАЯ ПЛАТА", "income", "salary"}, {"З/П", "income", "salary"}, {"Salary", "income", "salary"},
		{"Кафе", "income", "other"}, {"Зарплата", "expense", "other"},
		{"Подарки", "expense", "gifts"}, {"Подарки", "income", "gift-income"},
		{"Наличные", "account", "cash"}, {"НАЛИЧКА", "account", "cash"}, {"Cash RUB", "account", "cash"},
		{"Карта", "account", "debit-card"}, {"КАРТОЧКА", "account", "debit-card"}, {"Debit card", "account", "debit-card"},
		{"Кредитная карта", "account", "credit-card"}, {"КОШЕЛЁК", "account", "wallet"},
		{"Картография", "account", "wallet"}, {"Абракадабра", "account", "wallet"}, {"Абракадабра", "expense", "other"},
	}
	path := filepath.Join(s.T().TempDir(), "names.db")
	d, err := sql.Open("sqlite", path)
	s.Require().NoError(err)
	fixture, err := os.ReadFile("../importer/monefy/testdata/v11.sql")
	s.Require().NoError(err)
	_, err = d.Exec(string(fixture))
	s.Require().NoError(err)
	_, err = d.Exec(`UPDATE Account SET InitialBalanceCents=1000 WHERE Id='travel'`)
	s.Require().NoError(err)
	ids := make([]string, len(cases))
	for i, tc := range cases {
		ids[i] = uuid.NewString()
		if tc.kind == "account" {
			_, err = d.Exec(`INSERT INTO Account VALUES (?, ?, 1, 638396640000000000, 0, 1, 643, NULL, NULL)`, ids[i], tc.name)
		} else {
			categoryType := 1
			if tc.kind == "income" {
				categoryType = 0
			}
			_, err = d.Exec(`INSERT INTO Category VALUES (?, ?, ?, 1, NULL, NULL)`, ids[i], tc.name, categoryType)
		}
		s.Require().NoError(err)
	}
	s.Require().NoError(d.Close())
	f, err := os.Open(path)
	s.Require().NoError(err)
	defer f.Close() //nolint:errcheck
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Currencies(gomock.Any()).Return([]string{"RUB", "TRY"}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), false).Return(nil, nil)
	s.store.EXPECT().Save(gomock.Any()).Return(nil)
	p, err := s.svc.Preview(context.Background(), s.user, f, model.ImportEmpty)
	s.Require().NoError(err)
	actual := map[string]string{}
	for _, a := range p.Accounts {
		actual[a.SourceID] = a.Icon
	}
	for _, c := range p.Categories {
		actual[c.SourceID] = c.Icon
	}
	for i, tc := range cases {
		s.Run(tc.kind+"/"+tc.name, func() {
			parts := strings.Split(actual[ids[i]], "|")
			s.Require().Len(parts, 3)
			s.Equal("preset:"+tc.preset, parts[0])
			s.Contains([]string{"blue", "purple", "pink", "red", "orange", "green", "yellow", "graphite"}, parts[1])
			s.Equal(parts[1], parts[2])
		})
	}
}

func (s *ImportSuite) TestAccountAppearanceIsImmutableAndConfirmed() {
	p := model.ImportPreview{ID: s.id, UserID: s.user, Report: monefy.Report{
		Accounts: []monefy.Account{{ID: "a", Name: "Наличные", Currency: "RUB", InitialBalance: 12345}},
	}, Accounts: []model.ImportAccount{{SourceID: "a", Kind: "spending", Icon: "preset:cash|green|green", Balance: "12.345"}}, AccountIcons: map[string]string{"a": "preset:cash|green|green"}}
	chosen := "preset:wallet|pink|none"
	s.store.EXPECT().Load(s.user, s.id).Return(p, nil)
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), false).Return(nil, nil)
	s.store.EXPECT().Save(gomock.Any()).Return(nil)
	configured, err := s.svc.Configure(context.Background(), s.user, s.id, map[string]model.AccountKind{"a": "deposit"}, nil, map[string]string{"a": chosen}, nil)
	s.Require().NoError(err)
	s.Equal("preset:cash|green|green", p.AccountIcons["a"])
	s.Equal("preset:cash|green|green", p.Accounts[0].Icon)
	s.Equal(chosen, configured.Accounts[0].Icon)
	s.Equal(p.Accounts[0].Balance, configured.Accounts[0].Balance)
	s.Equal(p.Report.Accounts, configured.Report.Accounts)
	s.store.EXPECT().Load(s.user, configured.ID).Return(configured, nil)
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), false).Return(nil, nil)
	s.store.EXPECT().Save(gomock.Any()).Return(nil)
	again, err := s.svc.Configure(context.Background(), s.user, configured.ID, map[string]model.AccountKind{"a": "investment"}, nil, nil, nil)
	s.Require().NoError(err)
	s.Equal(chosen, again.Accounts[0].Icon)
	s.repo.EXPECT().LockUser(gomock.Any(), s.user).Return(nil)
	s.repo.EXPECT().Receipt(gomock.Any(), s.user, again.ID).Return(model.ImportResult{}, apperr.ErrNotFound)
	s.store.EXPECT().Load(s.user, again.ID).Return(again, nil)
	s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
	s.repo.EXPECT().Catalog(gomock.Any(), true).Return(nil, nil)
	s.repo.EXPECT().Currencies(gomock.Any()).Return([]string{"RUB"}, nil)
	s.repo.EXPECT().Write(gomock.Any(), again).Return(model.ImportResult{PreviewID: again.ID}, nil)
	s.store.EXPECT().Delete(s.user, again.ID).Return(nil)
	_, err = s.svc.Confirm(context.Background(), s.user, again.ID, true, false)
	s.Require().NoError(err)
}

func (s *ImportSuite) TestInvalidAccountIcons() {
	for _, tc := range []struct{ id, icon string }{
		{"a", "💰"}, {"a", "custom:AB"}, {"a", "preset:unknown"}, {"a", "preset:salary"},
		{"a", "preset:cash|neon|blue"}, {"a", "preset:cash|blue|blue|extra"}, {"missing", "preset:cash"},
	} {
		s.Run(tc.id+tc.icon, func() {
			p := model.ImportPreview{Report: monefy.Report{Accounts: []monefy.Account{{ID: "a"}}}}
			s.store.EXPECT().Load(s.user, s.id).Return(p, nil)
			s.repo.EXPECT().Availability(gomock.Any(), s.user).Return(model.ImportAvailability{}, nil)
			_, err := s.svc.Configure(context.Background(), s.user, s.id, map[string]model.AccountKind{"a": "spending"}, nil, map[string]string{tc.id: tc.icon}, nil)
			s.ErrorIs(err, apperr.ErrValidation)
			var typed *ImportError
			s.Require().ErrorAs(err, &typed)
			s.Equal("invalid_account_icons", typed.Code)
		})
	}
}

func (s *ImportSuite) TestReplacementRequiresSeparateAcknowledgement() {
	s.confirmStart(model.ImportPreview{Mode: model.ImportReplace})
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, true, false)
	s.ErrorContains(err, "deletion_not_confirmed")
}
func (s *ImportSuite) TestReplacementRejectsMissingManifest() {
	s.confirmStart(model.ImportPreview{Mode: model.ImportReplace})
	s.repo.EXPECT().LockReplacement(gomock.Any()).Return(nil)
	s.repo.EXPECT().Replacement(gomock.Any(), s.user).Return(model.ImportReplacement{}, nil)
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, true, true)
	s.ErrorContains(err, "replacement_changed")
}
func (s *ImportSuite) TestReplacementChecksBlockersEvenWithoutDiagnostics() {
	scope := model.ImportReplacement{Fingerprint: "snapshot", Blockers: map[string]int{"foreign_shares": 1}}
	s.confirmStart(model.ImportPreview{Mode: model.ImportReplace, Replacement: &scope})
	s.repo.EXPECT().LockReplacement(gomock.Any()).Return(nil)
	s.repo.EXPECT().Replacement(gomock.Any(), s.user).Return(scope, nil)
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, true, true)
	s.ErrorContains(err, "replacement_blocked")
}
func (s *ImportSuite) TestInvalidImportModeDoesNotParseOrDelete() {
	_, err := s.svc.Preview(context.Background(), s.user, strings.NewReader(""), "typo")
	s.ErrorContains(err, "invalid_import_mode")
}
func (s *ImportSuite) TestReplacementRejectsForeignPreview() {
	s.repo.EXPECT().LockUser(gomock.Any(), s.user).Return(nil)
	s.repo.EXPECT().Receipt(gomock.Any(), s.user, s.id).Return(model.ImportResult{}, apperr.ErrNotFound)
	s.store.EXPECT().Load(s.user, s.id).Return(model.ImportPreview{}, apperr.ErrNotFound)
	_, err := s.svc.Confirm(context.Background(), s.user, s.id, true, true)
	s.ErrorIs(err, apperr.ErrNotFound)
}
