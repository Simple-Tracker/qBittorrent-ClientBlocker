package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func installWebUITest(t *testing.T, peers map[string]WebUIBlockPeer) *Config {
	t.Helper()
	old := dependenciesSnapshot()
	cfg := &Config{Enabled: true}
	Configure(Dependencies{
		Config: func() Config { return *cfg },
		BlockStats: func() (int, int) {
			ports := 0
			for _, peer := range peers {
				ports += len(peer.Ports)
			}
			return len(peers), ports
		},
		BlockPeers: func() []WebUIBlockPeer {
			result := make([]WebUIBlockPeer, 0, len(peers))
			for _, peer := range peers {
				peer.Ports = append([]string{}, peer.Ports...)
				result = append(result, peer)
			}
			return result
		},
		BlockPeer: func(ip string) (WebUIBlockPeer, bool) {
			peer, exists := peers[ip]
			peer.Ports = append([]string{}, peer.Ports...)
			return peer, exists
		},
	})
	ResetWebUIPeerSync()
	t.Cleanup(func() { Configure(old); ResetWebUIPeerSync() })
	return cfg
}

func TestWebUIGetPeersSyncReturnsFullThenDeltas(t *testing.T) {
	peers := map[string]WebUIBlockPeer{"203.0.113.1": {IP: "203.0.113.1", Timestamp: 10, Ports: []string{"6881"}}}
	installWebUITest(t, peers)
	request := func(query string) WebUIBlockPeerSyncResponse {
		rec := httptest.NewRecorder()
		WebUI_GetPeers(rec, httptest.NewRequest(http.MethodGet, "/api/peers?"+query, nil))
		var response WebUIBlockPeerSyncResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	initial := request("sync=1")
	if !initial.Reset || initial.Cursor != 0 || len(initial.Peers) != 1 {
		t.Fatalf("initial: %+v", initial)
	}
	peers["203.0.113.2"] = WebUIBlockPeer{IP: "203.0.113.2", Timestamp: 20, Module: "BTN", Ports: []string{"ALL"}}
	WebUI_RecordBlockPeerAdded("203.0.113.2")
	added := request("sync=1&cursor=0")
	if added.Reset || added.Cursor != 1 || len(added.Peers) != 1 || added.Peers[0].IP != "203.0.113.2" {
		t.Fatalf("added: %+v", added)
	}
	delete(peers, "203.0.113.1")
	WebUI_RecordBlockPeerRemoved("203.0.113.1")
	removed := request("sync=1&cursor=1")
	if removed.Reset || removed.Cursor != 2 || len(removed.Peers) != 0 || len(removed.RemovedIP) != 1 || removed.RemovedIP[0] != "203.0.113.1" {
		t.Fatalf("removed: %+v", removed)
	}
}

func TestWebUIGetPeersSyncResetsExpiredCursor(t *testing.T) {
	installWebUITest(t, map[string]WebUIBlockPeer{"203.0.113.8": {IP: "203.0.113.8", Timestamp: 80}})
	webUIPeerSyncCursor = 10
	webUIPeerSyncEvents = []webUIBlockPeerEvent{{Cursor: 8, RemovedIP: "203.0.113.7"}}
	response := GetWebUIBlockPeerSync("3")
	if !response.Reset || response.Cursor != 10 || len(response.Peers) != 1 || response.Peers[0].IP != "203.0.113.8" {
		t.Fatalf("expired cursor: %+v", response)
	}
}

func TestWebUIPeerEventsAreDisabledWithWebUI(t *testing.T) {
	cfg := installWebUITest(t, nil)
	cfg.Enabled = false
	webUIPeerSyncCursor = 7
	WebUI_RecordBlockPeerAdded("203.0.113.1")
	WebUI_RecordBlockPeerRemoved("203.0.113.1")
	if webUIPeerSyncCursor != 7 || len(webUIPeerSyncEvents) != 0 {
		t.Fatal("disabled WebUI retained events")
	}
}

func TestWebUIPeerSyncCoalescesLatestState(t *testing.T) {
	installWebUITest(t, map[string]WebUIBlockPeer{"203.0.113.40": {IP: "203.0.113.40", Timestamp: 40, Reason: "latest"}})
	AppendWebUIBlockPeerEvent(webUIBlockPeerEvent{Peer: &WebUIBlockPeer{IP: "203.0.113.40", Timestamp: 10, Reason: "old"}})
	AppendWebUIBlockPeerEvent(webUIBlockPeerEvent{RemovedIP: "203.0.113.40"})
	AppendWebUIBlockPeerEvent(webUIBlockPeerEvent{Peer: &WebUIBlockPeer{IP: "203.0.113.40", Timestamp: 40, Reason: "latest"}})
	response := GetWebUIBlockPeerSync("0")
	if response.Reset || response.Cursor != 3 || len(response.Peers) != 1 || len(response.RemovedIP) != 0 || response.Peers[0].Reason != "latest" {
		t.Fatalf("coalesced: %+v", response)
	}
	if invalid := GetWebUIBlockPeerSync("not-a-cursor"); !invalid.Reset || len(invalid.Peers) != 1 {
		t.Fatalf("invalid cursor: %+v", invalid)
	}
}

func TestWebUIProvidersAndAuthentication(t *testing.T) {
	cfg := installWebUITest(t, nil)
	cfg.Username, cfg.Password = "user", "password"
	for _, valid := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		if valid {
			req.SetBasicAuth("user", "password")
		}
		rec := httptest.NewRecorder()
		if got := WebUI_CheckBasicAuth(rec, req); got != valid {
			t.Fatalf("authentication=%v, want %v", got, valid)
		}
		if !valid && (rec.Code != http.StatusUnauthorized || !json.Valid(rec.Body.Bytes())) {
			t.Fatalf("invalid auth response: %d %s", rec.Code, rec.Body.String())
		}
	}
	value := dependenciesSnapshot()
	value.Status = func() StatusResponse { return StatusResponse{ProgramName: "injected"} }
	value.LegacyLogs = func() []string { return []string{"legacy log"} }
	Configure(value)
	rec := httptest.NewRecorder()
	WebUI_GetStatus(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var status StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil || status.ProgramName != "injected" {
		t.Fatalf("status callback: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	WebUI_GetLogs(rec, httptest.NewRequest(http.MethodGet, "/api/logs", nil))
	var logs []string
	if err := json.Unmarshal(rec.Body.Bytes(), &logs); err != nil || len(logs) != 1 || logs[0] != "legacy log" {
		t.Fatalf("log callback: %s", rec.Body.String())
	}
}
