package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"maps"
	"math/big"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/co-wallet/backend/internal/apperr"
	"github.com/co-wallet/backend/internal/db"
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/importer/preview"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:generate mockgen -source=monefy_import.go -destination=mocks/mock_import.go -package=mocks
type importRepo interface {
	CurrencyRates(context.Context) (map[string]string, error)
	GetByUsername(context.Context, string) (model.User, error)
	LockUser(context.Context, string) error
	LockReplacement(context.Context) error
	Replacement(context.Context, string) (model.ImportReplacement, error)
	DeleteReplacement(context.Context, string) error
	AccountNames(context.Context, string, bool) ([]string, error)
	Currencies(context.Context) ([]string, error)
	Catalog(context.Context, bool) ([]model.Category, error)
	Receipt(context.Context, string, string) (model.ImportResult, error)
	Write(context.Context, model.ImportPreview) (model.ImportResult, error)
}
type importStore interface {
	Save(model.ImportPreview) error
	Load(string, string) (model.ImportPreview, error)
	Delete(string, string) error
}
type ImportService struct {
	repo   importRepo
	store  importStore
	withTx func(context.Context, func(importRepo) error) error
}

var importParseSlots = make(chan struct{}, 2)

func NewImportService(pool *pgxpool.Pool, repo *repository.ImportRepository, store *preview.Store) *ImportService {
	return &ImportService{repo: repo, store: store, withTx: func(ctx context.Context, fn func(importRepo) error) error {
		return db.WithTx(ctx, pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL READ COMMITTED"); err != nil {
				return err
			}
			return fn(repo.WithTx(tx))
		})
	}}
}

// ImportError exposes only stable, non-financial codes at the HTTP boundary.
type ImportError struct {
	Code string
	Kind error
}

func (e *ImportError) Error() string            { return e.Code }
func (e *ImportError) Unwrap() error            { return e.Kind }
func importError(code string, kind error) error { return &ImportError{Code: code, Kind: kind} }

func (s *ImportService) Availability(_ context.Context, user string) (model.ImportAvailability, error) {
	if _, err := uuid.Parse(user); err != nil {
		return model.ImportAvailability{}, apperr.ErrUnauthorized
	}
	return model.ImportAvailability{Reasons: []string{}}, nil
}

func (s *ImportService) Preview(ctx context.Context, user string, src io.Reader, mode model.ImportMode) (model.ImportPreview, error) {
	select {
	case importParseSlots <- struct{}{}:
		defer func() { <-importParseSlots }()
	default:
		return model.ImportPreview{}, importError("import_busy", apperr.ErrConflict)
	}
	if _, err := uuid.Parse(user); err != nil {
		return model.ImportPreview{}, apperr.ErrUnauthorized
	}
	if mode == "" {
		mode = model.ImportEmpty
	}
	if mode != model.ImportEmpty && mode != model.ImportReplace {
		return model.ImportPreview{}, importError("invalid_import_mode", apperr.ErrValidation)
	}
	currencies, err := s.repo.Currencies(ctx)
	if err != nil {
		return model.ImportPreview{}, err
	}
	hash := sha256.New()
	report, err := monefy.Parse(ctx, io.TeeReader(src, hash), monefy.Options{SupportedCurrencies: currencies})
	if err != nil {
		var parseErr *monefy.Error
		if errors.As(err, &parseErr) {
			return model.ImportPreview{}, importError("source_"+parseErr.Code, errors.Join(apperr.ErrValidation, err))
		}
		return model.ImportPreview{}, err
	}
	p := model.ImportPreview{Mode: mode, ID: uuid.NewString(), UserID: user, SHA256: hex.EncodeToString(hash.Sum(nil)), ExpiresAt: time.Now().UTC().Add(preview.TTL), Report: report}
	kinds := make(map[string]model.AccountKind, len(report.Accounts))
	for _, account := range report.Accounts {
		kinds[account.ID] = model.AccountKindSpending
	}
	return s.prepare(ctx, p, kinds)
}

// Configure creates a new immutable snapshot. Previously returned IDs keep their
// original parameters, so confirmation cannot race with a mutable options form.
func (s *ImportService) Configure(ctx context.Context, user, id string, kinds map[string]model.AccountKind, categoryIcons, accountIcons map[string]string, access map[string]model.ImportAccountAccess) (model.ImportPreview, error) {
	p, err := s.store.Load(user, id)
	if err != nil {
		return p, err
	}
	if len(kinds) != len(p.Report.Accounts) {
		return model.ImportPreview{}, importError("invalid_account_kinds", apperr.ErrValidation)
	}
	for _, a := range p.Report.Accounts {
		if !kinds[a.ID].IsValid() {
			return model.ImportPreview{}, importError("invalid_account_kinds", apperr.ErrValidation)
		}
	}
	icons := maps.Clone(p.CategoryIcons)
	if icons == nil {
		icons = map[string]string{}
	}
	for sourceID, icon := range categoryIcons {
		found := false
		for _, category := range p.Categories {
			if category.SourceID == sourceID && category.ExistingID == "" {
				found = true
				break
			}
		}
		if !found || !validImportIcon(icon, importCategoryPresets) {
			return model.ImportPreview{}, importError("invalid_category_icons", apperr.ErrValidation)
		}
		icons[sourceID] = icon
	}
	p.AccountIcons = maps.Clone(p.AccountIcons)
	if p.AccountIcons == nil {
		p.AccountIcons = map[string]string{}
	}
	for sourceID, icon := range accountIcons {
		if _, found := kinds[sourceID]; !found || !validImportIcon(icon, importAccountPresets) {
			return model.ImportPreview{}, importError("invalid_account_icons", apperr.ErrValidation)
		}
		p.AccountIcons[sourceID] = icon
	}
	p.AccountAccess = maps.Clone(p.AccountAccess)
	if p.AccountAccess == nil {
		p.AccountAccess = map[string]model.ImportAccountAccess{}
	}
	for sourceID, config := range access {
		if _, found := kinds[sourceID]; !found {
			return model.ImportPreview{}, importError("invalid_account_access", apperr.ErrValidation)
		}
		p.AccountAccess[sourceID] = config
	}
	p.CategoryIcons = icons
	p.ID = uuid.NewString()
	return s.prepare(ctx, p, kinds)
}

func (s *ImportService) prepare(ctx context.Context, p model.ImportPreview, kinds map[string]model.AccountKind) (model.ImportPreview, error) {
	catalog, err := s.repo.Catalog(ctx, false)
	if err != nil {
		return model.ImportPreview{}, err
	}
	// Rebuild API diagnostics while retaining the parser's original diagnostics.
	diagnostics := []monefy.Diagnostic{}
	for _, d := range p.Report.Diagnostics {
		if !strings.HasPrefix(d.Code, "target_") {
			diagnostics = append(diagnostics, d)
		}
	}
	p.Report.Diagnostics = diagnostics
	if err := prepareImportCurrency(ctx, s.repo, &p); err != nil {
		return model.ImportPreview{}, err
	}
	if p.Mode == model.ImportReplace {
		scope, err := s.repo.Replacement(ctx, p.UserID)
		if err != nil {
			return model.ImportPreview{}, err
		}
		p.Replacement = &scope
		for _, code := range []string{"shared_accounts", "foreign_membership", "foreign_members", "external_transactions", "foreign_authors", "foreign_shares"} {
			if scope.Blockers[code] > 0 {
				p.Report.Diagnostics = append(p.Report.Diagnostics, monefy.Diagnostic{Severity: monefy.Blocking, Code: "target_replace_" + code, Message: replacementReason(code)})
			}
		}
	}
	// Clone maps so preparing a new snapshot never mutates an older one.
	p.CategoryIcons = maps.Clone(p.CategoryIcons)
	if p.CategoryIcons == nil {
		p.CategoryIcons = map[string]string{}
	}
	for _, c := range matchImportCategories(p.Report.Categories, catalog) {
		if _, found := p.CategoryIcons[c.SourceID]; !found && c.ExistingID == "" {
			p.CategoryIcons[c.SourceID] = suggestImportIcon(c.Name, c.Type)
		}
	}
	p.AccountIcons = maps.Clone(p.AccountIcons)
	if p.AccountIcons == nil {
		p.AccountIcons = map[string]string{}
	}
	// Preserve appearance even for snapshots created before account options existed.
	for _, a := range p.Accounts {
		if _, found := p.AccountIcons[a.SourceID]; !found && a.Icon != "" {
			p.AccountIcons[a.SourceID] = a.Icon
		}
	}
	names, err := importAccountNames(ctx, s.repo, p, false)
	if err != nil {
		return model.ImportPreview{}, err
	}
	p.Accounts = []model.ImportAccount{}
	p.Categories = importCategoriesWithIcons(p.Report.Categories, catalog, p.CategoryIcons)
	add := func(severity monefy.Severity, code, entity, id, message string) {
		p.Report.Diagnostics = append(p.Report.Diagnostics, monefy.Diagnostic{Severity: severity, Code: "target_" + code, Entity: entity, SourceID: id, Message: message})
	}
	balances := map[string]*big.Int{}
	const maxAmount monefy.Amount = 99999999999999 // NUMERIC(15,4): at most 11 integer digits.
	checkAmount := func(a monefy.Amount, entity, id string, positive bool) {
		if a > maxAmount || a < -maxAmount || (positive && a <= 0) {
			add(monefy.Blocking, "amount_range", entity, id, "Сумма не помещается в co-wallet или равна нулю")
		}
	}
	for _, a := range p.Report.Accounts {
		balances[a.ID] = big.NewInt(int64(a.InitialBalance))
		checkAmount(a.InitialBalance, "Account", a.ID, false)
		if utf8.RuneCountInString(a.Name) > 100 {
			add(monefy.Blocking, "name_length", "Account", a.ID, "Имя длиннее 100 символов")
		}
		if !kinds[a.ID].IsValid() {
			add(monefy.Blocking, "account_kind", "Account", a.ID, "Выберите тип средств: текущие средства, сбережения, вклад, накопительный счёт или инвестиции")
		}
		if _, found := p.AccountIcons[a.ID]; !found {
			p.AccountIcons[a.ID] = suggestImportIcon(a.Name, "account")
		}
		p.Accounts = append(p.Accounts, model.ImportAccount{SourceID: a.ID, Name: names[a.ID], Kind: kinds[a.ID], Icon: p.AccountIcons[a.ID]})
	}
	seen := map[string]bool{}
	for _, c := range p.Categories {
		key := categoryKey(c.Type, c.Name)
		if seen[key] || c.ExistingID == "ambiguous" {
			add(monefy.Blocking, "ambiguous_category", "Category", c.SourceID, "Несколько категорий имеют одинаковые тип и нормализованное имя")
		}
		seen[key] = true
		if utf8.RuneCountInString(c.Name) > 100 {
			add(monefy.Blocking, "name_length", "Category", c.SourceID, "Имя длиннее 100 символов")
		}
	}
	p.PeriodFrom = nil
	p.PeriodTo = nil
	date := func(d time.Time) {
		if p.PeriodFrom == nil || d.Before(*p.PeriodFrom) {
			v := d
			p.PeriodFrom = &v
		}
		if p.PeriodTo == nil || d.After(*p.PeriodTo) {
			v := d
			p.PeriodTo = &v
		}
	}
	accountDates := map[string]time.Time{}
	for _, a := range p.Report.Accounts {
		accountDates[a.ID] = a.CreatedAt.Truncate(24 * time.Hour)
	}
	for _, t := range p.Report.Transactions {
		amount := t.Amount
		if amount < 0 {
			amount = -amount
		}
		checkAmount(amount, "Transaction", t.ID, true)
		if t.DefaultCurrencyAmount != nil {
			checkAmount(*t.DefaultCurrencyAmount, "Transaction", t.ID, true)
		}
		date(t.CreatedAt)
		if t.CreatedAt.Before(accountDates[t.AccountID]) {
			add(monefy.Blocking, "before_initial_balance", "Transaction", t.ID, "Операция раньше даты начального остатка; co-wallet не учтёт её в балансе")
		}
		if b := balances[t.AccountID]; b != nil {
			b.Add(b, big.NewInt(int64(t.Amount)))
		}
	}
	for _, t := range p.Report.Transfers {
		checkAmount(t.FromAmount, "Transfer", t.ID, true)
		date(t.CreatedAt)
		if t.CreatedAt.Before(accountDates[t.FromAccountID]) || t.CreatedAt.Before(accountDates[t.ToAccountID]) {
			add(monefy.Blocking, "before_initial_balance", "Transfer", t.ID, "Перевод раньше даты начального остатка")
		}
		if b := balances[t.FromAccountID]; b != nil {
			b.Sub(b, big.NewInt(int64(t.FromAmount)))
		}
		if t.ToAmount != nil {
			checkAmount(*t.ToAmount, "Transfer", t.ID, true)
			if b := balances[t.ToAccountID]; b != nil {
				b.Add(b, big.NewInt(int64(*t.ToAmount)))
			}
		}
	}
	for i := range p.Accounts {
		p.Accounts[i].Balance = new(big.Rat).SetFrac(balances[p.Accounts[i].SourceID], big.NewInt(1000)).FloatString(3)
	}
	if err = prepareImportShares(ctx, s.repo, &p); err != nil {
		return model.ImportPreview{}, err
	}
	add(monefy.Warning, "icons", "", "", "Иконки новых счетов и категорий подобраны по названию, цвета выбраны случайно из палитры co-wallet. Оформление можно изменить до подтверждения. Иконки общих категорий сохраняются; подбор выполняется локально на сервере")
	add(monefy.Warning, "flags", "", "", "Счета по умолчанию личные; совместный доступ задаётся явно до импорта. Все счета станут активными. IsIncludedInTotalBalance и disabled не переносятся: общий баланс определяется выбранным kind; история отключённых сущностей сохраняется")
	if err = s.store.Save(p); err != nil {
		if errors.Is(err, preview.ErrCapacity) {
			return model.ImportPreview{}, importError("preview_storage_full", errors.Join(apperr.ErrConflict, err))
		}
		return model.ImportPreview{}, err
	}
	return p, nil
}

func importCategoriesWithIcons(source []monefy.Category, catalog []model.Category, icons map[string]string) []model.ImportCategory {
	categories := matchImportCategories(source, catalog)
	for i := range categories {
		if icon, ok := icons[categories[i].SourceID]; ok && categories[i].ExistingID == "" {
			categories[i].Icon = icon
		}
	}
	return categories
}

func categoryKey(kind, name string) string {
	return kind + "\x00" + strings.ToLower(strings.TrimSpace(name))
}
func matchImportCategories(source []monefy.Category, catalog []model.Category) []model.ImportCategory {
	byKey := map[string][]model.Category{}
	for _, c := range catalog {
		key := categoryKey(string(c.Type), c.Name)
		byKey[key] = append(byKey[key], c)
	}
	out := []model.ImportCategory{}
	for _, c := range source {
		m := model.ImportCategory{SourceID: c.ID, Name: strings.TrimSpace(c.Name), Type: c.Type, Icon: "preset:other"}
		matches := byKey[categoryKey(c.Type, c.Name)]
		if len(matches) > 1 {
			m.ExistingID = "ambiguous"
		} else if len(matches) == 1 {
			m.ExistingID = matches[0].ID
			if matches[0].Icon != nil {
				m.Icon = *matches[0].Icon
			}
		}
		out = append(out, m)
	}
	return out
}

func (s *ImportService) Confirm(ctx context.Context, user, id string, acknowledgeExclusions, acknowledgeDeletion bool) (model.ImportResult, error) {
	if _, err := uuid.Parse(user); err != nil {
		return model.ImportResult{}, apperr.ErrUnauthorized
	}
	if _, err := uuid.Parse(id); err != nil {
		return model.ImportResult{}, apperr.ErrNotFound
	}
	var result model.ImportResult
	err := s.withTx(ctx, func(r importRepo) error {
		if err := r.LockUser(ctx, user); err != nil {
			return err
		}
		var err error
		result, err = r.Receipt(ctx, user, id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, apperr.ErrNotFound) {
			return err
		}
		p, err := s.store.Load(user, id)
		if err != nil {
			return err
		}
		for _, d := range p.Report.Diagnostics {
			if d.Severity == monefy.Blocking {
				return importError("preview_blocked", apperr.ErrValidation)
			}
			if d.Severity == monefy.Confirmation && !acknowledgeExclusions {
				return importError("exclusions_not_confirmed", apperr.ErrValidation)
			}
		}
		if len(p.Report.Exclusions) > 0 && !acknowledgeExclusions {
			return importError("exclusions_not_confirmed", apperr.ErrValidation)
		}
		if p.Mode == model.ImportReplace {
			if !acknowledgeDeletion {
				return importError("deletion_not_confirmed", apperr.ErrValidation)
			}
			if err = r.LockReplacement(ctx); err != nil {
				return err
			}
			scope, err := r.Replacement(ctx, user)
			if err != nil {
				return err
			}
			// Compare canonical DB content, not time.Time internals changed by the
			// preview's JSON round-trip (UTC versus a fixed-offset location).
			if p.Replacement == nil || p.Replacement.Fingerprint == "" || p.Replacement.Fingerprint != scope.Fingerprint {
				return importError("replacement_changed", apperr.ErrConflict)
			}
			for _, count := range scope.Blockers {
				if count > 0 {
					return importError("replacement_blocked", apperr.ErrConflict)
				}
			}
		}
		names, err := importAccountNames(ctx, r, p, true)
		if err != nil {
			return err
		}
		for _, account := range p.Accounts {
			if account.Name != names[account.SourceID] {
				return importError("account_names_changed", apperr.ErrConflict)
			}
		}
		catalog, err := r.Catalog(ctx, true)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(p.Categories, importCategoriesWithIcons(p.Report.Categories, catalog, p.CategoryIcons)) {
			return importError("catalog_changed", apperr.ErrConflict)
		}
		currencies, err := r.Currencies(ctx)
		if err != nil {
			return err
		}
		allowed := map[string]bool{}
		for _, c := range currencies {
			allowed[c] = true
		}
		for i, a := range p.Report.Accounts {
			if !allowed[a.Currency] || !p.Accounts[i].Kind.IsValid() {
				return importError("preview_stale", apperr.ErrConflict)
			}
		}
		if err = validateImportMembers(ctx, r, p); err != nil {
			return err
		}
		if p.Mode == model.ImportReplace {
			if err = r.DeleteReplacement(ctx, user); err != nil {
				return err
			}
		}
		result, err = r.Write(ctx, p)
		return err
	})
	if err != nil {
		return model.ImportResult{}, err
	}
	// A cleanup failure must not turn a committed import into an apparent failure.
	// TTL sweep retries deletion; receipt lookup makes all retries idempotent.
	_ = s.store.Delete(user, id) // При сбое очистки TTL-процесс повторит удаление; импорт уже зафиксирован.
	return result, nil
}

func replacementReason(code string) string {
	switch code {
	case "shared_accounts":
		return "У вас есть общие счета, включая удалённые. Полная замена недоступна."
	case "foreign_membership":
		return "Вы участвуете в чужих счетах. Полная замена недоступна."
	case "foreign_members":
		return "В ваших счетах участвуют другие пользователи. Полная замена недоступна."
	case "external_transactions":
		return "Есть операции или переводы, связанные с чужими счетами. Полная замена недоступна."
	case "foreign_authors":
		return "В вашей истории есть операции других пользователей. Полная замена недоступна."
	case "foreign_shares":
		return "В вашей истории есть доли других пользователей. Полная замена недоступна."
	default:
		return "Есть внешние связи. Полная замена недоступна."
	}
}
