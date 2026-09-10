package monefyhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/co-wallet/backend/internal/handler/monefy/mocks"
	"github.com/co-wallet/backend/internal/importer/monefy"
	"github.com/co-wallet/backend/internal/middleware"
	"github.com/co-wallet/backend/internal/model"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestHTTPValidation(t *testing.T) {
	for _, tt := range []struct {
		name, route, body, contentType string
		length                         int64
		status                         int
	}{
		{"csv", "/preview", "a,b", "text/csv", 3, 415},
		{"oversize", "/preview", "", "application/octet-stream", monefy.MaxFileBytes + 1, 413},
		{"bad json", "/id/confirm", "{", "application/json", 1, 400},
		{"unknown option", "/id/confirm", `{"account_kinds":{}}`, "application/json", 20, 400},
		{"trailing json", "/id/confirm", `{} {}`, "application/json", 5, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := New(mocks.NewMockimportService(gomock.NewController(t)))
			r := chi.NewRouter()
			r.Post("/preview", h.Preview)
			r.Post("/{previewID}/confirm", h.Confirm)
			req := httptest.NewRequest("POST", tt.route, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			req.ContentLength = tt.length
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, tt.status, w.Code)
		})
	}
}
func TestAvailabilityAndConfirmContract(t *testing.T) {
	svc := mocks.NewMockimportService(gomock.NewController(t))
	h := New(svc)
	r := chi.NewRouter()
	r.Get("/availability", h.Availability)
	r.Post("/{previewID}/confirm", h.Confirm)
	svc.EXPECT().Availability(gomock.Any(), "user").Return(model.ImportAvailability{Reasons: []string{"owned_accounts"}}, nil)
	req := httptest.NewRequest("GET", "/availability", nil).WithContext(context.WithValue(context.Background(), middleware.ContextUserID, "user"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.JSONEq(t, `{"available":false,"reasons":["owned_accounts"]}`, w.Body.String())
	svc.EXPECT().Confirm(gomock.Any(), "user", "id", true, false).Return(model.ImportResult{PreviewID: "id", Transactions: 2}, nil)
	req = httptest.NewRequest("POST", "/id/confirm", strings.NewReader(`{"acknowledge_exclusions":true}`)).WithContext(req.Context())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"transactions":2`)
}

func TestOptionsAppearanceContract(t *testing.T) {
	svc := mocks.NewMockimportService(gomock.NewController(t))
	h := New(svc)
	r := chi.NewRouter()
	r.Post("/{previewID}/options", h.Configure)
	svc.EXPECT().Configure(gomock.Any(), "user", "id", map[string]model.AccountKind{"a": "spending"}, map[string]string{"c": "preset:cafe|red|none"}, map[string]string{"a": "preset:cash|pink|pink"}, map[string]model.ImportAccountAccess{}).Return(model.ImportPreview{ID: "new"}, nil)
	req := httptest.NewRequest("POST", "/id/options", strings.NewReader(`{"account_kinds":{"a":"spending"},"category_icons":{"c":"preset:cafe|red|none"},"account_icons":{"a":"preset:cash|pink|pink"}}`)).WithContext(context.WithValue(context.Background(), middleware.ContextUserID, "user"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	require.Contains(t, w.Body.String(), `"preview_id":"new"`)
}

func TestSharedOptionsContract(t *testing.T) {
	svc := mocks.NewMockimportService(gomock.NewController(t))
	h := New(svc)
	r := chi.NewRouter()
	r.Post("/{previewID}/options", h.Configure)
	expected := map[string]model.ImportAccountAccess{"a": {AccessMode: "shared", Members: []model.CreateAccountMemberReq{{Username: "owner", DefaultShare: .6}, {Username: "other", DefaultShare: .4}}}}
	svc.EXPECT().Configure(gomock.Any(), "user", "id", map[string]model.AccountKind{"a": "spending"}, gomock.Any(), gomock.Any(), expected).Return(model.ImportPreview{ID: "new"}, nil)
	req := httptest.NewRequest("POST", "/id/options", strings.NewReader(`{"account_kinds":{"a":"spending"},"account_access":{"a":{"access_mode":"shared","members":[{"username":"owner","default_share":0.6},{"username":"other","default_share":0.4}]}}}`)).WithContext(context.WithValue(context.Background(), middleware.ContextUserID, "user"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
}

func TestCurrencyRatesContract(t *testing.T) {
	svc := mocks.NewMockimportService(gomock.NewController(t))
	r := chi.NewRouter()
	r.Post("/{previewID}/rates", New(svc).ConfigureRates)
	svc.EXPECT().ConfigureRates(gomock.Any(), "user", "id", map[string]string{"TRY": "2.5"}).Return(model.ImportPreview{
		ID: "new", CurrencyRates: map[string]model.ImportCurrencyRate{"TRY": {Currency: "TRY", BaseCurrency: "RUB", Rate: "2.5", Source: "manual", Transactions: 12}},
	}, nil)
	req := httptest.NewRequest("POST", "/id/rates", strings.NewReader(`{"rates":{"TRY":"2.5"}}`)).WithContext(context.WithValue(context.Background(), middleware.ContextUserID, "user"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	require.Contains(t, w.Body.String(), `"preview_id":"new"`)
	require.Contains(t, w.Body.String(), `"currency_rates":[{"currency":"TRY","base_currency":"RUB","rate":"2.5","source":"manual","transactions":12}]`)
}
