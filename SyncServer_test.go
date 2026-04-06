package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncWithServerPrepareJSON(t *testing.T) {
	oldConfig := *config
	oldCurrentTimestamp := currentTimestamp
	defer func() {
		tmpConf := oldConfig
		config = &tmpConf
		currentTimestamp = oldCurrentTimestamp
	}()

	tmpConf := oldConfig
	config = &tmpConf
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
	oldConfig := *config
	oldSyncConfig := syncServer_syncConfig
	oldRules := syncServer_CompiledRules
	defer func() {
		httpClientExternal = oldClientExternal
		tmpConf := oldConfig
		config = &tmpConf
		syncServer_syncConfig = oldSyncConfig
		syncServer_CompiledRules = oldRules
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
	tmpConf := oldConfig
	config = &tmpConf
	config.SyncServerURL = server.URL
	syncServer_syncConfig = &SyncServer_ConfigStruct{
		Interval:    60,
		Status:      "",
		BlockIPRule: map[string][]string{},
	}
	syncServer_CompiledRules = []SyncServer_RuleStruct{}

	if !SyncWithServer_Submit(`{"dummy":true}`) {
		t.Fatal("SyncWithServer_Submit should succeed")
	}
	if syncServer_syncConfig.Interval != 90 {
		t.Fatalf("Interval=%d, want 90", syncServer_syncConfig.Interval)
	}
	if len(syncServer_CompiledRules) != 2 {
		t.Fatalf("compiled Rule count=%d, want 2", len(syncServer_CompiledRules))
	}

	foundIPv4 := false
	foundIPv6 := false
	for _, rule := range syncServer_CompiledRules {
		if rule.Net.String() == "1.2.3.4/32" {
			foundIPv4 = true
		}
		if rule.Net.String() == "2001:db8::/64" {
			foundIPv6 = true
		}
	}
	if !foundIPv4 {
		t.Fatal("compiled Rules should contain IPv4 entry")
	}
	if !foundIPv6 {
		t.Fatal("compiled Rules should contain IPv6 entry")
	}
}

func TestSyncServerCheckPeer(t *testing.T) {
	oldRules := syncServer_CompiledRules
	defer func() {
		syncServer_CompiledRules = oldRules
	}()

	_, subnet, _ := net.ParseCIDR("10.0.0.0/24")
	syncServer_CompiledRules = []SyncServer_RuleStruct{
		{Net: subnet, Reason: "local-network"},
	}

	ok, reason := SyncServer_CheckPeer(net.ParseIP("10.0.0.5"))
	if !ok || reason != "Bad-IP_FromSyncServer (local-network)" {
		t.Fatalf("SyncServer_CheckPeer(10.0.0.5) = (%v, %q), want (true, \"Bad-IP_FromSyncServer (local-network)\")", ok, reason)
	}

	ok, _ = SyncServer_CheckPeer(net.ParseIP("10.0.1.5"))
	if ok {
		t.Fatal("SyncServer_CheckPeer(10.0.1.5) should be false")
	}
}
