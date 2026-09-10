package repository_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	accounthandler "github.com/co-wallet/backend/internal/handler/account"
	categoryhandler "github.com/co-wallet/backend/internal/handler/category"
	monefyhandler "github.com/co-wallet/backend/internal/handler/monefy"
	transactionhandler "github.com/co-wallet/backend/internal/handler/transaction"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
	"github.com/co-wallet/backend/internal/repository"
	"github.com/co-wallet/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

// Uses real HTTP handlers, JWT middleware, services, SQLite and PostgreSQL.
// PostgreSQL isolation and migrations are shared with the atomic import tests.
func (f *importFixture) api(t *testing.T) (http.Handler, string) {
	t.Helper()
	users := repository.NewUserRepository(f.pool)
	accounts := repository.NewAccountRepository(f.pool)
	auth := service.NewAuthService(users, "synthetic-integration-secret")
	tokens, err := auth.IssueTokens(model.User{ID: f.user})
	require.NoError(t, err)
	h := monefyhandler.New(f.svc)
	a := accounthandler.New(service.NewAccountService(f.pool, accounts, users), service.NewUserService(users))
	tx := transactionhandler.New(service.NewTransactionService(f.pool, repository.NewTransactionRepository(f.pool), accounts, repository.NewTagRepository(f.pool)))
	c := categoryhandler.New(service.NewCategoryService(repository.NewCategoryRepository(f.pool)))
	r := chi.NewRouter()
	r.Use(middleware.Auth(auth))
	r.Get("/api/imports/monefy/availability", h.Availability)
	r.Post("/api/imports/monefy/preview", h.Preview)
	r.Post("/api/imports/monefy/{previewID}/options", h.Configure)
	r.Post("/api/imports/monefy/{previewID}/confirm", h.Confirm)
	r.Get("/api/accounts", a.List)
	r.Post("/api/accounts", a.Create)
	r.Route("/api/accounts/{accountID}", func(r chi.Router) {
		r.Use(middleware.AccountMember(accounts))
		r.Get("/", a.Get)
		r.Patch("/", a.Update)
		r.Get("/members", a.ListMembers)
		r.Post("/members", a.AddMember)
		r.Patch("/members/{userID}", a.UpdateMember)
		r.Delete("/members/{userID}", a.RemoveMember)
	})
	r.Get("/api/categories", c.List)
	r.Get("/api/transactions", tx.List)
	r.Get("/api/transactions/{transactionID}", tx.Get)
	return r, tokens.AccessToken
}

func importRequest(t *testing.T, h http.Handler, token, method, path string, body []byte, status int, target any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if strings.HasPrefix(path, "/api/imports/monefy/preview") {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// Never include a response body: the opt-in reference test uses personal data.
	require.Equal(t, status, w.Code, "unexpected HTTP status")
	if target != nil {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), target))
	}
}

type apiPreview struct {
	ID                 string         `json:"preview_id"`
	CanConfirm         bool           `json:"can_confirm"`
	RequiresExclusions bool           `json:"requires_exclusion_confirmation"`
	Counts             map[string]int `json:"counts"`
	Accounts           []struct {
		ID         string `json:"source_id"`
		Kind       string `json:"kind"`
		Name       string `json:"name"`
		SourceName string `json:"source_name"`
		Balance    string `json:"final_balance"`
	} `json:"accounts"`
	Exclusions []struct {
		ID string `json:"source_id"`
	} `json:"exclusions"`
}

func TestMonefyAPIEndToEnd(t *testing.T) {
	for _, override := range []bool{false, true} {
		name := "defaults"
		if override {
			name = "override_one_account"
		}
		t.Run(name, func(t *testing.T) { testMonefyAPIEndToEnd(t, override) })
	}
}

func testMonefyAPIEndToEnd(t *testing.T, override bool) {
	f := newImportFixture(t)
	src, err := sql.Open("sqlite", f.source)
	require.NoError(t, err)
	fixture, err := os.ReadFile("../importer/monefy/testdata/api.sql")
	require.NoError(t, err)
	_, err = src.Exec(string(fixture))
	require.NoError(t, err)
	require.NoError(t, src.Close())
	_, err = f.pool.Exec(context.Background(), `INSERT INTO categories(user_id,name,type,icon) VALUES($1,'FOOD','expense','preset:cafe')`, f.other)
	require.NoError(t, err)
	h, token := f.api(t)
	source, err := os.ReadFile(f.source)
	require.NoError(t, err)
	var p apiPreview
	importRequest(t, h, token, "POST", "/api/imports/monefy/preview", source, 201, &p)
	require.True(t, p.CanConfirm)
	for _, account := range p.Accounts {
		require.Equal(t, "spending", account.Kind)
	}
	require.True(t, p.RequiresExclusions)
	require.Equal(t, map[string]int{"accounts": 3, "categories": 3, "transactions": 3, "transfers": 2}, p.Counts)
	require.Len(t, p.Exclusions, 1)
	require.Equal(t, "orphan", p.Exclusions[0].ID)
	expectedBalances := map[string]string{"Cash": "-3728.391", "Travel": "1412.520", "Reserve": "1234.567"}
	for _, a := range p.Accounts {
		require.Equal(t, expectedBalances[a.Name], a.Balance)
	}
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/confirm", []byte(`{}`), 400, nil)
	if override {
		importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/options", []byte(`{"account_kinds":{"cash":"spending","travel":"deposit","reserve":"spending"}}`), 201, &p)
	}
	require.True(t, p.CanConfirm)
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/confirm", []byte(`{}`), 400, nil)
	var result map[string]any
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/confirm", []byte(`{"acknowledge_exclusions":true}`), 200, &result)
	require.Equal(t, float64(2), result["categories"])
	require.Equal(t, float64(1), result["reused_categories"])
	var repeated map[string]any
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/confirm", []byte(`{}`), 200, &repeated)
	require.Equal(t, result, repeated)
	var accounts []accounthandler.AccountResponse
	importRequest(t, h, token, "GET", "/api/accounts?currency=RUB", nil, 200, &accounts)
	require.Len(t, accounts, 3)
	for _, account := range accounts {
		want := "spending"
		if override && account.Name == "Travel" {
			want = "deposit"
		}
		require.Equal(t, want, account.Kind)
	}
	expected := map[string]struct {
		initial, balance float64
		currency         string
	}{
		"Cash":    {-1234.567, -3728.391, "RUB"},
		"Travel":  {1, 1412.520, "TRY"},
		"Reserve": {0, 1234.567, "RUB"},
	}
	for _, a := range accounts {
		e, ok := expected[a.Name]
		require.True(t, ok)
		require.Equal(t, e.initial, a.InitialBalance)
		require.NotNil(t, a.Balance)
		require.Equal(t, e.balance, a.Balance.Native)
		require.Equal(t, e.balance, a.Balance.TotalNative)
		require.Equal(t, e.currency, a.Currency)
		require.Equal(t, "personal", a.AccessMode)
	}
	var categories []categoryhandler.CategoryResponse
	importRequest(t, h, token, "GET", "/api/categories?type=expense", nil, 200, &categories)
	require.Len(t, categories, 2)
	names := map[string]bool{}
	for _, c := range categories {
		names[c.Name] = true
		if c.Name == "FOOD" {
			require.Equal(t, f.other, c.UserID)
			require.Equal(t, "preset:cafe", *c.Icon)
		}
	}
	require.True(t, names["Unused"])
	var txs []transactionhandler.TransactionResponse
	importRequest(t, h, token, "GET", "/api/transactions?limit=100", nil, 200, &txs)
	require.Len(t, txs, 5)
	counts := map[string]int{}
	for _, tx := range txs {
		var detail transactionhandler.TransactionResponse
		importRequest(t, h, token, "GET", "/api/transactions/"+tx.ID, nil, 200, &detail)
		require.Equal(t, tx.ID, detail.ID)
		require.Len(t, detail.Shares, 1)
		require.Equal(t, f.user, detail.Shares[0].UserID)
		require.Equal(t, detail.Amount, detail.Shares[0].Amount)
		require.False(t, detail.Shares[0].IsCustom)
		counts[tx.Type]++
		if tx.Type == "expense" {
			require.Equal(t, 12.345, tx.Amount)
			require.Equal(t, "Identical note", *tx.Description)
			require.Equal(t, "2024-01-02", tx.Date.Format("2006-01-02"))
		}
		if tx.Type == "income" {
			require.Equal(t, 999.999, tx.Amount)
		}
		if tx.Type == "transfer" {
			require.Equal(t, 1234.567, tx.Amount)
			require.NotNil(t, tx.ToAmount)
			if tx.ToCurrency == "TRY" {
				require.Equal(t, 411.521, *tx.ToAmount)
			} else {
				require.Equal(t, 1234.567, *tx.ToAmount)
			}
		}
	}
	require.Equal(t, map[string]int{"expense": 2, "income": 1, "transfer": 2}, counts)
	var availability struct {
		Available bool `json:"available"`
	}
	importRequest(t, h, token, "GET", "/api/imports/monefy/availability", nil, 200, &availability)
	require.True(t, availability.Available)
	var added apiPreview
	importRequest(t, h, token, "POST", "/api/imports/monefy/preview", source, 201, &added)
	for _, a := range added.Accounts {
		require.Equal(t, a.SourceName+" (1)", a.Name)
	}
	after, err := os.ReadFile(f.source)
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(source), sha256.Sum256(after))
}

// Opt-in end-to-end reconciliation. Only counts and boolean comparisons may be
// logged: CSV and SQLite exports contain private financial records.
func TestMonefyAPILocalReference(t *testing.T) {
	dbPath, csvPath := os.Getenv("MONEFY_REFERENCE_DB"), os.Getenv("MONEFY_REFERENCE_CSV")
	if dbPath == "" || csvPath == "" {
		t.Skip("local reference exports not configured")
	}
	f := newImportFixture(t)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO currencies(code,name) VALUES('LKR','Rupee'),('ILS','Shekel') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	source, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	csvSource, err := os.ReadFile(csvPath)
	require.NoError(t, err)
	rows, err := csv.NewReader(bytes.NewReader(csvSource)).ReadAll()
	require.NoError(t, err)
	expectedBalances, expectedTotals := map[string]*big.Rat{}, map[string]*big.Rat{}
	add := func(m map[string]*big.Rat, key, value string) {
		v, ok := new(big.Rat).SetString(value)
		require.True(t, ok, "invalid decimal")
		if m[key] == nil {
			m[key] = new(big.Rat)
		}
		m[key].Add(m[key], v)
	}
	for _, r := range rows[1:] {
		require.Equal(t, 8, len(r))
		add(expectedBalances, r[1]+"\x00"+r[4], r[3])
		kind := r[2]
		if kind != "InitialBalance" && kind != "IncomeTransfer" && kind != "ExpenseTransfer" {
			if len(r[3]) > 0 && r[3][0] == '-' {
				kind = "expense"
			} else {
				kind = "income"
			}
		}
		add(expectedTotals, r[4]+"/"+kind, r[3])
	}
	h, token := f.api(t)
	var p apiPreview
	importRequest(t, h, token, "POST", "/api/imports/monefy/preview", source, 201, &p)
	require.Equal(t, map[string]int{"accounts": 17, "categories": 42, "transactions": 2557, "transfers": 273}, p.Counts)
	require.Equal(t, 1, len(p.Exclusions))
	require.True(t, p.RequiresExclusions)
	kinds := map[string]string{}
	for _, a := range p.Accounts {
		kinds[a.ID] = "spending"
	}
	options, err := json.Marshal(map[string]any{"account_kinds": kinds})
	require.NoError(t, err)
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/options", options, 201, &p)
	require.True(t, p.CanConfirm, "reference preview blocked")
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/confirm", []byte(`{"acknowledge_exclusions":true}`), 200, nil)
	var accounts []accounthandler.AccountResponse
	importRequest(t, h, token, "GET", "/api/accounts?currency=RUB", nil, 200, &accounts)
	require.Equal(t, 17, len(accounts))
	actualBalances, actualTotals := map[string]*big.Rat{}, map[string]*big.Rat{}
	number := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	nonzero := 0
	for _, a := range accounts {
		require.NotNil(t, a.Balance)
		add(actualBalances, a.Name+"\x00"+a.Currency, number(a.Balance.Native))
		if a.InitialBalance != 0 {
			nonzero++
			add(actualTotals, a.Currency+"/InitialBalance", number(a.InitialBalance))
		}
	}
	require.Equal(t, 13, nonzero)
	ordinary, transfers, fx := 0, 0, 0
	for page := 1; page <= 30; page++ {
		var txs []transactionhandler.TransactionResponse
		importRequest(t, h, token, "GET", "/api/transactions?limit=100&page="+strconv.Itoa(page), nil, 200, &txs)
		if len(txs) == 0 {
			break
		}
		for _, tx := range txs {
			require.True(t, len(tx.Shares) == 1 && tx.Shares[0].UserID == f.user && tx.Shares[0].Amount == tx.Amount, "share mismatch")
			if tx.Type == "transfer" {
				transfers++
				require.NotNil(t, tx.ToAmount)
				if tx.Currency != tx.ToCurrency {
					fx++
				}
				add(actualTotals, tx.Currency+"/ExpenseTransfer", number(-tx.Amount))
				add(actualTotals, tx.ToCurrency+"/IncomeTransfer", number(*tx.ToAmount))
			} else {
				ordinary++
				amount := tx.Amount
				if tx.Type == "expense" {
					amount = -amount
				}
				add(actualTotals, tx.Currency+"/"+tx.Type, number(amount))
			}
		}
	}
	require.Equal(t, 2557, ordinary)
	require.Equal(t, 273, transfers)
	require.Equal(t, 10, fx)
	compare := func(expected, actual map[string]*big.Rat) {
		require.True(t, len(expected) == len(actual), "aggregate key count mismatch")
		mismatches := 0
		for key, value := range expected {
			if actual[key] == nil || actual[key].Cmp(value) != 0 {
				mismatches++
			}
		}
		require.Zero(t, mismatches, "exact decimal aggregate mismatch (private values omitted)")
	}
	compare(expectedBalances, actualBalances)
	compare(expectedTotals, actualTotals)
	after, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	require.True(t, sha256.Sum256(source) == sha256.Sum256(after), "DB changed")
	afterCSV, err := os.ReadFile(csvPath)
	require.NoError(t, err)
	require.True(t, sha256.Sum256(csvSource) == sha256.Sum256(afterCSV), "CSV changed")
	t.Log("verified API import: 17 accounts, 42 categories, 2557 operations, 273 transfers (10 FX), 13 nonzero opening balances, 1 acknowledged exclusion; exact per-account balances and per-currency totals match CSV; both sources unchanged")
}

func TestMonefyReplacementAPIContract(t *testing.T) {
	f := newImportFixture(t)
	a, tx := f.oldHistory(t)
	h, token := f.api(t)
	source, err := os.ReadFile(f.source)
	require.NoError(t, err)
	var p struct {
		ID          string `json:"preview_id"`
		Mode        string `json:"mode"`
		CanConfirm  bool   `json:"can_confirm"`
		Replacement struct {
			Counts   map[string]int `json:"counts"`
			Accounts []struct {
				ID string `json:"id"`
			} `json:"accounts"`
		} `json:"replacement"`
	}
	importRequest(t, h, token, "POST", "/api/imports/monefy/preview", source, 201, nil)
	importRequest(t, h, token, "POST", "/api/imports/monefy/preview?mode=invalid", source, 400, nil)
	importRequest(t, h, token, "POST", "/api/imports/monefy/preview?mode=replace", source, 201, &p)
	require.Equal(t, "replace", p.Mode)
	require.True(t, p.CanConfirm)
	require.Equal(t, 1, p.Replacement.Counts["deleted_accounts"])
	require.Equal(t, a, p.Replacement.Accounts[0].ID)
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/confirm", []byte(`{"acknowledge_exclusions":true}`), 400, nil)
	f.exists(t, a, tx)
	var result, repeat map[string]any
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/confirm", []byte(`{"acknowledge_deletion":true}`), 200, &result)
	importRequest(t, h, token, "POST", "/api/imports/monefy/"+p.ID+"/confirm", []byte(`{}`), 200, &repeat)
	require.Equal(t, result, repeat)
}
