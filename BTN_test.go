package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTorrentIdentifierIsStableAndCaseInsensitive(t *testing.T) {
	id1 := GetTorrentIdentifier("ABCDEF0123456789")
	id2 := GetTorrentIdentifier("abcdef0123456789")

	if id1 != id2 {
		t.Fatalf("GetTorrentIdentifier should be case-insensitive: %q != %q", id1, id2)
	}
	if len(id1) != 64 {
		t.Fatalf("identifier length=%d, want 64", len(id1))
	}
	if id1 == "abcdef0123456789" {
		t.Fatal("identifier should not equal the raw torrent hash")
	}
}

func TestBTNGetConfigLoadsConfiguration(t *testing.T) {
	oldClientExternal := httpClientExternal
	oldConfig := config
	oldBtnConfig := btnConfig
	oldLastGetConfig := btn_lastGetConfig
	oldCurrentTimestamp := currentTimestamp
	oldIsGetting := btn_isGettingConfig.Load()
	defer func() {
		httpClientExternal = oldClientExternal
		config = oldConfig
		btnConfig = oldBtnConfig
		btn_lastGetConfig = oldLastGetConfig
		currentTimestamp = oldCurrentTimestamp
		btn_isGettingConfig.Store(oldIsGetting)
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer app-a@secret-a" {
			t.Fatalf("Authorization=%q, want Bearer app-a@secret-a", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"min_protocol_version": 1,
			"max_protocol_version": 3,
			"ability": {
				"submit_peers": {
					"interval": 10,
					"endpoint": "https://btn.example/submit_peers",
					"random_initial_delay": 0,
					"version": "v1"
				}
			}
		}`))
	}))
	defer server.Close()

	httpClientExternal = *server.Client()
	config = oldConfig
	config.BTNConfigureURL = server.URL
	config.BTNAppID = "app-a"
	config.BTNAppSecret = "secret-a"
	btnConfig = nil
	btn_lastGetConfig = 0
	currentTimestamp = 100
	btn_isGettingConfig.Store(false)

	BTN_GetConfig()

	if btnConfig == nil {
		t.Fatal("BTN_GetConfig should populate btnConfig")
	}
	if btnConfig.MinMainVersion != 1 || btnConfig.MaxMainVersion != 3 {
		t.Fatalf("unexpected protocol range: min=%d max=%d", btnConfig.MinMainVersion, btnConfig.MaxMainVersion)
	}
	ability, ok := btnConfig.Ability["submit_peers"]
	if !ok {
		t.Fatal("submit_peers ability should exist")
	}
	if ability.Endpoint != "https://btn.example/submit_peers" {
		t.Fatalf("Endpoint=%q, want https://btn.example/submit_peers", ability.Endpoint)
	}
	if btn_lastGetConfig != 100 {
		t.Fatalf("btn_lastGetConfig=%d, want 100", btn_lastGetConfig)
	}
}
