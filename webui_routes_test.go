package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebUIV1RoutesAndErrors(t *testing.T) {
	old := config
	cfg := *ConfigSnapshot()
	cfg.WebUI = true
	cfg.WebUIUsername = "test-user"
	cfg.WebUIPassword = "test-password"
	config = &cfg
	t.Cleanup(func() { config = old })

	handler := &httpServerHandler{}
	for _, path := range []string{"/api/v1/status", "/api/v1/bans", "/api/v1/logs"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.SetBasicAuth(cfg.WebUIUsername, cfg.WebUIPassword)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || !json.Valid(rec.Body.Bytes()) {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}

	for _, tc := range []struct {
		method, path, code string
		status             int
	}{
		{http.MethodPost, "/api/v1/bans", "method_not_allowed", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/unknown", "not_found", http.StatusNotFound},
		{http.MethodGet, "/api/v1/status", "unauthorized", http.StatusUnauthorized},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rec.Code != tc.status || body.Error.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, rec.Code, rec.Body.String())
		}
	}

	cfg.WebUI = false
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if rec.Code != http.StatusNotFound || !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("disabled API: %d %s", rec.Code, rec.Body.String())
	}
}
