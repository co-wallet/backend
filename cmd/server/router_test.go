package main

import (
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestMonefyRequiresAuthentication(t *testing.T) {
	r := newRouter(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	for _, path := range []string{"/api/imports/monefy/availability", "/api/imports/monefy/preview", "/api/imports/monefy/id/options", "/api/imports/monefy/id/confirm"} {
		method := "POST"
		if path == "/api/imports/monefy/availability" {
			method = "GET"
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		require.Equal(t, 401, w.Code)
	}
}
