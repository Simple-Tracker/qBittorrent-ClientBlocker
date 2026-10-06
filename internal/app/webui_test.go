package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/webui"
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
	webui.WebUI_GetStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}

	var resp webui.StatusResponse
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
	webui.WebUI_GetPeers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}

	var peers []webui.WebUIBlockPeer
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

func TestAddBlockPeerRecordsUpdatedWebUIPeers(t *testing.T) {
	oldConfig := *config
	oldCurrentTimestamp := currentTimestamp
	blockPeerMapMutex.Lock()
	oldBlockPeerMap := blockPeerMap
	blockPeerMap = map[string]BlockPeerInfoStruct{}
	blockPeerMapMutex.Unlock()
	blockCIDRMapMutex.Lock()
	oldBlockCIDRMap := blockCIDRMap
	blockCIDRMap = map[string]BlockCIDRInfoStruct{}
	blockCIDRMapMutex.Unlock()
	webui.ResetWebUIPeerSync()
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		currentTimestamp = oldCurrentTimestamp
		blockPeerMapMutex.Lock()
		blockPeerMap = oldBlockPeerMap
		blockPeerMapMutex.Unlock()
		blockCIDRMapMutex.Lock()
		blockCIDRMap = oldBlockCIDRMap
		blockCIDRMapMutex.Unlock()
		webui.ResetWebUIPeerSync()
	})

	testConfig := oldConfig
	testConfig.WebUI = true
	testConfig.ExecCommand_Ban = ""
	config = &testConfig
	currentTimestamp = 10
	AddBlockPeer("CheckPeer", "first", "203.0.113.20", 6881, "hash", "id", "client", 1, 2)
	initial := webui.GetWebUIBlockPeerSync("")
	currentTimestamp = 20
	AddBlockPeer("CheckPeer", "updated", "203.0.113.20", 6882, "hash", "id", "client", 3, 4)
	AddBlockPeer("BTN", "latest", "203.0.113.20", 6882, "hash", "new-id", "new-client", 5, 6)

	response := webui.GetWebUIBlockPeerSync("1")
	if response.Reset || response.Cursor != 3 || len(response.Peers) != 1 || len(response.RemovedIP) != 0 {
		t.Fatalf("existing peer updates were not merged: %#v", response)
	}
	peer := response.Peers[0]
	// 更换 PeerID 后, 端口 6882 开始新会话: 1+3+5 / 2+4+6.
	if peer.Module != "BTN" || peer.Reason != "latest" || peer.ID != "new-id" || peer.Client != "new-client" || peer.Timestamp != 20 || peer.Downloaded != 9 || peer.Uploaded != 12 {
		t.Fatalf("delta contains stale peer data: %#v", peer)
	}
	if len(peer.Ports) != 2 || peer.Ports[0] != "6881" || peer.Ports[1] != "6882" {
		t.Fatalf("delta ports=%v", peer.Ports)
	}
	if initial.Cursor != 1 || len(initial.Peers[0].Ports) != 1 || initial.Peers[0].Uploaded != 2 {
		t.Fatal("updates mutated the previous snapshot")
	}
	if next := webui.GetWebUIBlockPeerSync("3"); len(next.Peers) != 0 || next.Reset {
		t.Fatalf("acknowledged updates were repeated: %#v", next)
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
