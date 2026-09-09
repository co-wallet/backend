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
	svc.EXPECT().Confirm(gomock.Any(), "user", "id", true).Return(model.ImportResult{PreviewID: "id", Transactions: 2}, nil)
	req = httptest.NewRequest("POST", "/id/confirm", strings.NewReader(`{"acknowledge_exclusions":true}`)).WithContext(req.Context())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"transactions":2`)
}
