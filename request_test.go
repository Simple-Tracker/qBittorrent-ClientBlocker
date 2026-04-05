package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewRequest_ClientHeadersAndAuth(t *testing.T) {
	oldConfig := config
	oldClientType := currentClientType
	oldTrToken := Tr_csrfToken
	defer func() {
		config = oldConfig
		currentClientType = oldClientType
		Tr_csrfToken = oldTrToken
	}()

	config = oldConfig
	config.UseBasicAuth = true
	config.ClientUsername = "user-a"
	config.ClientPassword = "pass-a"
	currentClientType = "Transmission"
	Tr_csrfToken = "csrf-a"

	req := NewRequest(true, "http://example.com", "a=1", true, false, nil)
	if req == nil {
		t.Fatalf("NewRequest returned nil")
	}
	if req.Header.Get("User-Agent") != programUserAgent {
		t.Fatalf("unexpected user-agent: %q", req.Header.Get("User-Agent"))
	}
	if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected content-type: %q", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("X-Transmission-Session-Id") != "csrf-a" {
		t.Fatalf("unexpected transmission csrf header: %q", req.Header.Get("X-Transmission-Session-Id"))
	}
	username, password, ok := req.BasicAuth()
	if !ok || username != "user-a" || password != "pass-a" {
		t.Fatalf("unexpected basic auth: ok=%v user=%q pass=%q", ok, username, password)
	}
}

func TestNewRequest_CacheHeaders(t *testing.T) {
	oldETag := urlETagCache
	oldLastMod := urlLastModCache
	defer func() {
		urlETagCache = oldETag
		urlLastModCache = oldLastMod
	}()

	urlETagCache = map[string]string{"http://example.com/rules": "etag-1"}
	urlLastModCache = map[string]string{"http://example.com/rules": "Mon, 01 Jan 2024 00:00:00 GMT"}

	req := NewRequest(false, "http://example.com/rules", "", false, true, nil)
	if req == nil {
		t.Fatalf("NewRequest returned nil")
	}
	if req.Header.Get("If-None-Match") != "etag-1" {
		t.Fatalf("unexpected If-None-Match: %q", req.Header.Get("If-None-Match"))
	}
	if req.Header.Get("If-Modified-Since") == "" {
		t.Fatalf("If-Modified-Since should be set")
	}
}

func TestFetch_CacheRoundTrip(t *testing.T) {
	oldClientExternal := httpClientExternal
	oldETag := urlETagCache
	oldLastMod := urlLastModCache
	defer func() {
		httpClientExternal = oldClientExternal
		urlETagCache = oldETag
		urlLastModCache = oldLastMod
	}()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("ETag", "etag-a")
			w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}

		if r.Header.Get("If-None-Match") != "etag-a" {
			t.Fatalf("If-None-Match not sent on cached request: %q", r.Header.Get("If-None-Match"))
		}
		if r.Header.Get("If-Modified-Since") == "" {
			t.Fatalf("If-Modified-Since not sent on cached request")
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	httpClientExternal = *server.Client()
	urlETagCache = map[string]string{}
	urlLastModCache = map[string]string{}

	code, _, body := Fetch(server.URL, false, false, true, nil)
	if code != http.StatusOK {
		t.Fatalf("first Fetch status=%d, want 200", code)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected first body: %q", string(body))
	}

	code, _, body = Fetch(server.URL, false, false, true, nil)
	if code != http.StatusNotModified {
		t.Fatalf("second Fetch status=%d, want 304", code)
	}
	if body != nil {
		t.Fatalf("second Fetch body should be nil for 304")
	}
}

func TestFetch_Transmission409SetsCSRF(t *testing.T) {
	oldClient := httpClient
	oldClientType := currentClientType
	oldToken := Tr_csrfToken
	defer func() {
		httpClient = oldClient
		currentClientType = oldClientType
		Tr_csrfToken = oldToken
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Transmission-Session-Id", "csrf-new")
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()

	httpClient = *server.Client()
	currentClientType = "Transmission"
	Tr_csrfToken = ""

	code, _, body := Fetch(server.URL, false, true, false, nil)
	if code != http.StatusConflict {
		t.Fatalf("Fetch status=%d, want 409", code)
	}
	if body != nil {
		t.Fatalf("body should be nil for 409 token bootstrap path")
	}
	if Tr_csrfToken != "csrf-new" {
		t.Fatalf("Tr_csrfToken=%q, want %q", Tr_csrfToken, "csrf-new")
	}
}

func TestNewRequest_CustomHeadersPreserved(t *testing.T) {
	custom := map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   "unit-test-agent",
	}
	req := NewRequest(true, "http://example.com", `{"a":1}`, false, false, &custom)
	if req == nil {
		t.Fatalf("NewRequest returned nil")
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content-type: %q", req.Header.Get("Content-Type"))
	}
	if !strings.Contains(req.Header.Get("User-Agent"), "unit-test-agent") {
		t.Fatalf("unexpected user-agent: %q", req.Header.Get("User-Agent"))
	}
}
