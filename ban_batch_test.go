package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestQBBanBatchesResumeAndSubmitOnlyChanges(t *testing.T) {
	oldCfg, oldHTTP, oldMethod := ConfigSnapshot(), httpClient, qB_useNewBanPeersMethod
	t.Cleanup(func() { ReplaceConfig(oldCfg); httpClient = oldHTTP; qB_useNewBanPeersMethod = oldMethod })
	var mu sync.Mutex
	requests, endpoints := 0, 0
	succeeded := make(map[string]bool)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if r.ContentLength > qBMaxBanFormBytes {
			t.Errorf("oversized form: %d", r.ContentLength)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		payload := r.Form.Get("peers")
		if requests == 2 {
			http.Error(w, "temporary failure", 503)
			return
		}
		if succeeded[payload] {
			t.Error("successfully acknowledged batch was resent")
		}
		succeeded[payload] = true
		endpoints += len(strings.Split(payload, "|"))
		w.Write([]byte("Ok."))
	}))
	defer server.Close()
	cfg := *oldCfg
	cfg.ClientURL, cfg.BanAllPort = server.URL, false
	ReplaceConfig(&cfg)
	httpClient = *server.Client()
	qB_useNewBanPeersMethod = true
	client := &QBClient{}
	peers := map[string]BlockPeerInfoStruct{"192.0.2.20": {Port: map[int]bool{-1: true}}}
	if client.SubmitBlockPeer(peers) {
		t.Fatal("failed batch was reported as successful")
	}
	if !client.SubmitBlockPeer(peers) {
		t.Fatal("retry did not finish remaining batches")
	}
	mu.Lock()
	count, total := requests, endpoints
	mu.Unlock()
	if total != 2*65536 {
		t.Fatalf("acknowledged endpoints=%d", total)
	}
	if !client.SubmitBlockPeer(peers) {
		t.Fatal("unchanged list failed")
	}
	mu.Lock()
	unchanged := requests
	mu.Unlock()
	if unchanged != count {
		t.Fatal("unchanged list generated a request")
	}
	peers["198.51.100.1"] = BlockPeerInfoStruct{Port: map[int]bool{6881: true}}
	if !client.SubmitBlockPeer(peers) {
		t.Fatal("new IP failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != count+1 || !succeeded["198.51.100.1:6881"] {
		t.Fatal("new IP was not submitted independently")
	}
}

func TestQBLegacyBanRetainsReplacementSemantics(t *testing.T) {
	oldCfg, oldHTTP, oldMethod := ConfigSnapshot(), httpClient, qB_useNewBanPeersMethod
	t.Cleanup(func() { ReplaceConfig(oldCfg); httpClient = oldHTTP; qB_useNewBanPeersMethod = oldMethod })
	var lists []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/app/setPreferences" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		var data map[string]string
		if err := json.Unmarshal([]byte(r.Form.Get("json")), &data); err != nil {
			t.Error(err)
		}
		lists = append(lists, data["banned_IPs"])
		w.Write([]byte("Ok."))
	}))
	defer server.Close()
	cfg := *oldCfg
	cfg.ClientURL = server.URL
	ReplaceConfig(&cfg)
	httpClient = *server.Client()
	qB_useNewBanPeersMethod = false
	client := &QBClient{}
	peers := map[string]BlockPeerInfoStruct{"192.0.2.1": {Port: map[int]bool{1: true}}}
	if !client.SubmitBlockPeer(peers) {
		t.Fatal("first submission failed")
	}
	peers["192.0.2.1"].Port[2] = true
	if !client.SubmitBlockPeer(peers) || len(lists) != 1 {
		t.Fatal("IP-only API resent a port-only change")
	}
	peers["198.51.100.1"] = BlockPeerInfoStruct{}
	if !client.SubmitBlockPeer(peers) || len(lists) != 2 || !strings.Contains(lists[1], "192.0.2.1\n") || !strings.Contains(lists[1], "198.51.100.1\n") {
		t.Fatal("replacement omitted existing IPs")
	}
	delete(peers, "192.0.2.1")
	if !client.SubmitBlockPeer(peers) || strings.Contains(lists[2], "192.0.2.1") {
		t.Fatal("replacement retained removed IP")
	}
}
