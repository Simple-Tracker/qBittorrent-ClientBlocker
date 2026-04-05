package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebUIBasicAuth(t *testing.T) {
	oldConfig := *config
	defer func() {
		tmpConf := oldConfig
	config = &tmpConf
	}()

	tmpConf := oldConfig
	config = &tmpConf
	config.WebUI = true
	config.WebUIUsername = "webui-user"
	config.WebUIPassword = "webui-pass"

	handler := &httpServerHandler{}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatalf("missing basic auth challenge: %q", rec.Header().Get("WWW-Authenticate"))
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.SetBasicAuth("webui-user", "webui-pass")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<!DOCTYPE html>") {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestWebUIBasicAuthDisabledWhenUsernameEmpty(t *testing.T) {
	oldConfig := *config
	defer func() {
		tmpConf := oldConfig
	config = &tmpConf
	}()

	tmpConf := oldConfig
	config = &tmpConf
	config.WebUI = true
	config.WebUIUsername = ""
	config.WebUIPassword = "ignored-password"

	handler := &httpServerHandler{}
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWebUIGetStatusCounts(t *testing.T) {
	oldBlockPeerMap := blockPeerMap
	oldCurrentTimestamp := currentTimestamp
	oldClientType := currentClientType
	oldBTNConfig := btnConfig
	oldConfig := *config
	defer func() {
		blockPeerMap = oldBlockPeerMap
		currentTimestamp = oldCurrentTimestamp
		currentClientType = oldClientType
		btnConfig = oldBTNConfig
		tmpConf := oldConfig
	config = &tmpConf
	}()

	blockPeerMap = map[string]BlockPeerInfoStruct{
		"1.2.3.4": {
			Timestamp: 10,
			Module:    "CheckPeer",
			Reason:    "Bad-Port",
			Port:      map[int]bool{6881: true, 6882: true},
		},
		"5.6.7.8": {
			Timestamp: 20,
			Module:    "CheckPeer",
			Reason:    "Bad-CIDR",
			Port:      map[int]bool{-1: true},
		},
	}
	currentTimestamp = 1234
	currentClientType = "qBittorrent"
	btnConfig = nil
	tmpConf := oldConfig
	config = &tmpConf
	config.SyncServerURL = "http://sync.example"

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/status", nil)
	rec := httptest.NewRecorder()
	WebUI_GetStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}

	var resp StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal status response: %v", err)
	}
	if resp.CurrentStats.TotalBlockedIPs != 2 {
		t.Fatalf("TotalBlockedIPs=%d, want 2", resp.CurrentStats.TotalBlockedIPs)
	}
	if resp.CurrentStats.TotalBlockedPorts != 3 {
		t.Fatalf("TotalBlockedPorts=%d, want 3", resp.CurrentStats.TotalBlockedPorts)
	}
	if resp.CurrentStats.LastUpdateTimestamp != 1234 {
		t.Fatalf("LastUpdateTimestamp=%d, want 1234", resp.CurrentStats.LastUpdateTimestamp)
	}
	if len(resp.LoadedExtensions) != 1 || resp.LoadedExtensions[0] != "SyncServer" {
		t.Fatalf("LoadedExtensions=%#v, want []string{\"SyncServer\"}", resp.LoadedExtensions)
	}
}

func TestWebUIGetPeersResponse(t *testing.T) {
	oldBlockPeerMap := blockPeerMap
	defer func() {
		blockPeerMap = oldBlockPeerMap
	}()

	blockPeerMap = map[string]BlockPeerInfoStruct{
		"1.2.3.4": {
			Timestamp: 10,
			Module:    "CheckPeer",
			Reason:    "Bad-Port",
			Port:      map[int]bool{6882: true, 6881: true},
		},
		"5.6.7.8": {
			Timestamp: 20,
			Module:    "CheckPeer",
			Reason:    "Bad-CIDR",
			Port:      map[int]bool{-1: true},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/peers", nil)
	rec := httptest.NewRecorder()
	WebUI_GetPeers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}

	var peers []WebUIBlockPeer
	if err := json.Unmarshal(rec.Body.Bytes(), &peers); err != nil {
		t.Fatalf("unmarshal peers response: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("len(peers)=%d, want 2", len(peers))
	}
	if peers[0].IP != "5.6.7.8" || peers[0].Timestamp != 20 {
		t.Fatalf("unexpected first peer: %#v", peers[0])
	}
	if len(peers[0].Ports) != 1 || peers[0].Ports[0] != "ALL" {
		t.Fatalf("unexpected ALL ports: %#v", peers[0].Ports)
	}
	if strings.Join(peers[1].Ports, ",") != "6881,6882" {
		t.Fatalf("unexpected sorted ports: %#v", peers[1].Ports)
	}
}

func TestFormatConfigValueForLog(t *testing.T) {
	if got := FormatConfigValueForLog("ClientPassword", "secret-a"); got != "[REDACTED]" {
		t.Fatalf("ClientPassword=%v, want [REDACTED]", got)
	}
	if got := FormatConfigValueForLog("BTNAppSecret", "secret-b"); got != "[REDACTED]" {
		t.Fatalf("BTNAppSecret=%v, want [REDACTED]", got)
	}
	if got := FormatConfigValueForLog("SyncServerToken", "token-a"); got != "[REDACTED]" {
		t.Fatalf("SyncServerToken=%v, want [REDACTED]", got)
	}
	if got := FormatConfigValueForLog("ClientUsername", "user-a"); got != "user-a" {
		t.Fatalf("ClientUsername=%v, want user-a", got)
	}
}
