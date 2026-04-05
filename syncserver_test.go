package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncWithServerPrepareJSON(t *testing.T) {
	oldConfig := config
	oldCurrentTimestamp := currentTimestamp
	defer func() {
		config = oldConfig
		currentTimestamp = oldCurrentTimestamp
	}()

	config = oldConfig
	config.SyncServerToken = "token-a"
	currentTimestamp = 123456

	torrentMapInput := map[string]TorrentInfoStruct{
		"hash-a": {
			Size: 100,
			Peers: map[string]PeerInfoStruct{
				"1.2.3.4": {Uploaded: 2048},
			},
		},
	}

	ok, payload := SyncWithServer_PrepareJSON(torrentMapInput)
	if !ok {
		t.Fatal("SyncWithServer_PrepareJSON should succeed")
	}

	var body SyncServer_SubmitStruct
	if err := json.Unmarshal([]byte(payload), &body); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if body.Version != 1 {
		t.Fatalf("Version=%d, want 1", body.Version)
	}
	if body.Timestamp != 123456 {
		t.Fatalf("Timestamp=%d, want 123456", body.Timestamp)
	}
	if body.Token != "token-a" {
		t.Fatalf("Token=%q, want token-a", body.Token)
	}
	if _, ok := body.TorrentMap["hash-a"]; !ok {
		t.Fatal("torrent payload should include hash-a")
	}
}

func TestSyncWithServerSubmitCompilesCIDRs(t *testing.T) {
	oldClientExternal := httpClientExternal
	oldConfig := config
	oldSyncConfig := syncServer_syncConfig
	oldCIDRMap := ipBlockCIDRMapFromSyncServerCompiled
	defer func() {
		httpClientExternal = oldClientExternal
		config = oldConfig
		syncServer_syncConfig = oldSyncConfig
		ipBlockCIDRMapFromSyncServerCompiled = oldCIDRMap
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"interval": 90,
			"status": "",
			"blockIPRule": {
				"bad-peer": ["1.2.3.4", "2001:db8::/64", "bad-cidr"]
			}
		}`))
	}))
	defer server.Close()

	httpClientExternal = *server.Client()
	config = oldConfig
	config.SyncServerURL = server.URL
	syncServer_syncConfig = SyncServer_ConfigStruct{
		Interval:    60,
		Status:      "",
		BlockIPRule: map[string][]string{},
	}
	ipBlockCIDRMapFromSyncServerCompiled = map[string]*net.IPNet{}

	if !SyncWithServer_Submit(`{"dummy":true}`) {
		t.Fatal("SyncWithServer_Submit should succeed")
	}
	if syncServer_syncConfig.Interval != 90 {
		t.Fatalf("Interval=%d, want 90", syncServer_syncConfig.Interval)
	}
	if len(ipBlockCIDRMapFromSyncServerCompiled) != 2 {
		t.Fatalf("compiled CIDR count=%d, want 2", len(ipBlockCIDRMapFromSyncServerCompiled))
	}
	if _, ok := ipBlockCIDRMapFromSyncServerCompiled["1.2.3.4"]; !ok {
		t.Fatal("compiled CIDRs should contain IPv4 entry")
	}
	if _, ok := ipBlockCIDRMapFromSyncServerCompiled["2001:db8::/64"]; !ok {
		t.Fatal("compiled CIDRs should contain IPv6 entry")
	}
}
