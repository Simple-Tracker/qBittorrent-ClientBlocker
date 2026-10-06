package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestWebUIGetPeersSyncReturnsFullThenDeltas(t *testing.T) {
	oldConfig := *config
	testConfig := oldConfig
	testConfig.WebUI = true
	testConfig.ExecCommand_Ban = ""
	config = &testConfig
	blockPeerMapMutex.Lock()
	oldBlockPeerMap := blockPeerMap
	blockPeerMap = map[string]BlockPeerInfoStruct{
		"203.0.113.1": {Timestamp: 10, Module: "CheckPeer", Reason: "Bad-Port", Port: map[int]bool{6881: true}},
	}
	blockPeerMapMutex.Unlock()
	webUIPeerSyncMutex.Lock()
	oldCursor := webUIPeerSyncCursor
	oldEvents := webUIPeerSyncEvents
	oldEventStart := webUIPeerSyncEventStart
	webUIPeerSyncCursor = 0
	webUIPeerSyncEvents = nil
	webUIPeerSyncEventStart = 0
	webUIPeerSyncMutex.Unlock()
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		blockPeerMapMutex.Lock()
		blockPeerMap = oldBlockPeerMap
		blockPeerMapMutex.Unlock()
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncCursor = oldCursor
		webUIPeerSyncEvents = oldEvents
		webUIPeerSyncEventStart = oldEventStart
		webUIPeerSyncMutex.Unlock()
	})

	requestSync := func(rawQuery string) WebUIBlockPeerSyncResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		WebUI_GetPeers(recorder, httptest.NewRequest(http.MethodGet, "http://example.com/api/peers?"+rawQuery, nil))
		var response WebUIBlockPeerSyncResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("unmarshal sync response: %v", err)
		}
		return response
	}

	initial := requestSync("sync=1")
	if !initial.Reset || initial.Cursor != 0 || len(initial.Peers) != 1 || initial.Peers[0].IP != "203.0.113.1" {
		t.Fatalf("unexpected initial sync: %#v", initial)
	}

	blockPeerMapMutex.Lock()
	blockPeerMap["203.0.113.2"] = BlockPeerInfoStruct{Timestamp: 20, Module: "BTN", Reason: "Reputation", Port: map[int]bool{-1: true}}
	blockPeerMapMutex.Unlock()
	WebUI_RecordBlockPeerAdded("203.0.113.2")
	added := requestSync("sync=1&cursor=0")
	if added.Reset || added.Cursor != 1 || len(added.Peers) != 1 || added.Peers[0].IP != "203.0.113.2" {
		t.Fatalf("unexpected add delta: %#v", added)
	}

	blockPeerMapMutex.Lock()
	delete(blockPeerMap, "203.0.113.1")
	blockPeerMapMutex.Unlock()
	WebUI_RecordBlockPeerRemoved("203.0.113.1")
	removed := requestSync("sync=1&cursor=1")
	if removed.Reset || removed.Cursor != 2 || len(removed.Peers) != 0 || len(removed.RemovedIP) != 1 || removed.RemovedIP[0] != "203.0.113.1" {
		t.Fatalf("unexpected remove delta: %#v", removed)
	}
}

func TestWebUIGetPeersSyncResetsExpiredCursor(t *testing.T) {
	blockPeerMapMutex.Lock()
	oldBlockPeerMap := blockPeerMap
	blockPeerMap = map[string]BlockPeerInfoStruct{
		"203.0.113.8": {Timestamp: 80, Port: map[int]bool{6881: true}},
	}
	blockPeerMapMutex.Unlock()
	webUIPeerSyncMutex.Lock()
	oldCursor := webUIPeerSyncCursor
	oldEvents := webUIPeerSyncEvents
	oldEventStart := webUIPeerSyncEventStart
	webUIPeerSyncCursor = 10
	webUIPeerSyncEvents = []webUIBlockPeerEvent{{Cursor: 8, RemovedIP: "203.0.113.7"}}
	webUIPeerSyncEventStart = 0
	webUIPeerSyncMutex.Unlock()
	t.Cleanup(func() {
		blockPeerMapMutex.Lock()
		blockPeerMap = oldBlockPeerMap
		blockPeerMapMutex.Unlock()
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncCursor = oldCursor
		webUIPeerSyncEvents = oldEvents
		webUIPeerSyncEventStart = oldEventStart
		webUIPeerSyncMutex.Unlock()
	})

	recorder := httptest.NewRecorder()
	WebUI_GetPeers(recorder, httptest.NewRequest(http.MethodGet, "http://example.com/api/peers?sync=1&cursor=3", nil))
	var response WebUIBlockPeerSyncResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Reset || response.Cursor != 10 || len(response.Peers) != 1 || response.Peers[0].IP != "203.0.113.8" {
		t.Fatalf("expired cursor did not receive a full reset: %#v", response)
	}
}

func TestWebUIPeerSyncEventBufferKeepsNewestEventsInOrder(t *testing.T) {
	webUIPeerSyncMutex.Lock()
	oldCursor := webUIPeerSyncCursor
	oldEvents := webUIPeerSyncEvents
	oldEventStart := webUIPeerSyncEventStart
	webUIPeerSyncCursor = 0
	webUIPeerSyncEvents = nil
	webUIPeerSyncEventStart = 0
	webUIPeerSyncMutex.Unlock()
	t.Cleanup(func() {
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncCursor = oldCursor
		webUIPeerSyncEvents = oldEvents
		webUIPeerSyncEventStart = oldEventStart
		webUIPeerSyncMutex.Unlock()
	})

	for index := 0; index < webUIMaxPeerEvents+2; index++ {
		AppendWebUIBlockPeerEvent(webUIBlockPeerEvent{RemovedIP: strconv.Itoa(index)})
	}

	webUIPeerSyncMutex.Lock()
	defer webUIPeerSyncMutex.Unlock()
	if len(webUIPeerSyncEvents) != webUIMaxPeerEvents {
		t.Fatalf("event count=%d, want %d", len(webUIPeerSyncEvents), webUIMaxPeerEvents)
	}
	if first := WebUIBlockPeerEventAt(0); first.Cursor != 3 || first.RemovedIP != "2" {
		t.Fatalf("oldest event=%#v", first)
	}
	if last := WebUIBlockPeerEventAt(len(webUIPeerSyncEvents) - 1); last.Cursor != webUIMaxPeerEvents+2 {
		t.Fatalf("newest event=%#v", last)
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
	webUIPeerSyncMutex.Lock()
	oldCursor := webUIPeerSyncCursor
	oldEvents := webUIPeerSyncEvents
	oldEventStart := webUIPeerSyncEventStart
	webUIPeerSyncCursor = 0
	webUIPeerSyncEvents = nil
	webUIPeerSyncEventStart = 0
	webUIPeerSyncMutex.Unlock()
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
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncCursor = oldCursor
		webUIPeerSyncEvents = oldEvents
		webUIPeerSyncEventStart = oldEventStart
		webUIPeerSyncMutex.Unlock()
	})

	testConfig := oldConfig
	testConfig.WebUI = true
	testConfig.ExecCommand_Ban = ""
	config = &testConfig
	currentTimestamp = 10
	AddBlockPeer("CheckPeer", "first", "203.0.113.20", 6881, "hash", "id", "client", 1, 2)
	initial := GetWebUIBlockPeerSync("")
	currentTimestamp = 20
	AddBlockPeer("CheckPeer", "updated", "203.0.113.20", 6882, "hash", "id", "client", 3, 4)
	AddBlockPeer("BTN", "latest", "203.0.113.20", 6882, "hash", "new-id", "new-client", 5, 6)

	response := GetWebUIBlockPeerSync("1")
	if response.Reset || response.Cursor != 3 || len(response.Peers) != 1 || len(response.RemovedIP) != 0 {
		t.Fatalf("existing peer updates were not merged: %#v", response)
	}
	peer := response.Peers[0]
	if peer.Module != "BTN" || peer.Reason != "latest" || peer.ID != "new-id" || peer.Client != "new-client" || peer.Timestamp != 20 || peer.Downloaded != 6 || peer.Uploaded != 8 {
		t.Fatalf("delta contains stale peer data: %#v", peer)
	}
	if len(peer.Ports) != 2 || peer.Ports[0] != "6881" || peer.Ports[1] != "6882" {
		t.Fatalf("delta ports=%v", peer.Ports)
	}
	if initial.Cursor != 1 || len(initial.Peers[0].Ports) != 1 || initial.Peers[0].Uploaded != 2 {
		t.Fatal("updates mutated the previous snapshot")
	}
	if next := GetWebUIBlockPeerSync("3"); len(next.Peers) != 0 || next.Reset {
		t.Fatalf("acknowledged updates were repeated: %#v", next)
	}
}

func TestWebUIPeerSyncResetsAfterRestart(t *testing.T) {
	webUIPeerSyncMutex.Lock()
	oldEpoch, oldCursor, oldEvents, oldStart := webUIPeerSyncEpoch, webUIPeerSyncCursor, webUIPeerSyncEvents, webUIPeerSyncEventStart
	webUIPeerSyncEpoch, webUIPeerSyncCursor, webUIPeerSyncEvents = "new-instance", 0, nil
	webUIPeerSyncMutex.Unlock()
	t.Cleanup(func() {
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncEpoch, webUIPeerSyncCursor, webUIPeerSyncEvents, webUIPeerSyncEventStart = oldEpoch, oldCursor, oldEvents, oldStart
		webUIPeerSyncMutex.Unlock()
	})
	response := GetWebUIBlockPeerSync("0", "old-instance")
	if !response.Reset || response.Epoch != "new-instance" {
		t.Fatalf("old instance cursor accepted: %#v", response)
	}
	response = GetWebUIBlockPeerSync("0", "new-instance")
	if response.Reset || len(response.Peers) != 0 {
		t.Fatal("unchanged current instance did not return empty delta")
	}
}

func TestWebUIPeerEventsAreDisabledWithWebUI(t *testing.T) {
	oldConfig := *config
	webUIPeerSyncMutex.Lock()
	oldCursor := webUIPeerSyncCursor
	oldEvents := webUIPeerSyncEvents
	oldEventStart := webUIPeerSyncEventStart
	webUIPeerSyncCursor = 7
	webUIPeerSyncEvents = nil
	webUIPeerSyncEventStart = 0
	webUIPeerSyncMutex.Unlock()
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncCursor = oldCursor
		webUIPeerSyncEvents = oldEvents
		webUIPeerSyncEventStart = oldEventStart
		webUIPeerSyncMutex.Unlock()
	})
	testConfig := oldConfig
	testConfig.WebUI = false
	config = &testConfig

	WebUI_RecordBlockPeerAdded("203.0.113.1")
	WebUI_RecordBlockPeerRemoved("203.0.113.1")
	webUIPeerSyncMutex.Lock()
	cursor := webUIPeerSyncCursor
	eventCount := len(webUIPeerSyncEvents)
	webUIPeerSyncMutex.Unlock()
	if cursor != 7 || eventCount != 0 {
		t.Fatalf("disabled WebUI recorded cursor=%d events=%d", cursor, eventCount)
	}
}

func TestWebUIPeerSyncCoalescesLatestState(t *testing.T) {
	blockPeerMapMutex.Lock()
	oldBlockPeerMap := blockPeerMap
	blockPeerMap = map[string]BlockPeerInfoStruct{
		"203.0.113.40": {Timestamp: 40, Reason: "latest", Port: map[int]bool{6881: true}},
	}
	blockPeerMapMutex.Unlock()
	webUIPeerSyncMutex.Lock()
	oldCursor := webUIPeerSyncCursor
	oldEvents := webUIPeerSyncEvents
	oldEventStart := webUIPeerSyncEventStart
	webUIPeerSyncCursor = 3
	webUIPeerSyncEvents = []webUIBlockPeerEvent{
		{Cursor: 1, Peer: &WebUIBlockPeer{IP: "203.0.113.40", Timestamp: 10, Reason: "old"}},
		{Cursor: 2, RemovedIP: "203.0.113.40"},
		{Cursor: 3, Peer: &WebUIBlockPeer{IP: "203.0.113.40", Timestamp: 40, Reason: "latest", Ports: []string{"6881"}}},
	}
	webUIPeerSyncEventStart = 0
	webUIPeerSyncMutex.Unlock()
	t.Cleanup(func() {
		blockPeerMapMutex.Lock()
		blockPeerMap = oldBlockPeerMap
		blockPeerMapMutex.Unlock()
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncCursor = oldCursor
		webUIPeerSyncEvents = oldEvents
		webUIPeerSyncEventStart = oldEventStart
		webUIPeerSyncMutex.Unlock()
	})

	response := GetWebUIBlockPeerSync("0")
	if response.Reset || response.Cursor != 3 || len(response.Peers) != 1 || len(response.RemovedIP) != 0 {
		t.Fatalf("coalesced response=%#v", response)
	}
	if response.Peers[0].Reason != "latest" || response.Peers[0].Timestamp != 40 {
		t.Fatalf("coalesced peer=%#v", response.Peers[0])
	}
	invalid := GetWebUIBlockPeerSync("not-a-cursor")
	if !invalid.Reset || len(invalid.Peers) != 1 {
		t.Fatalf("invalid cursor response=%#v", invalid)
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
