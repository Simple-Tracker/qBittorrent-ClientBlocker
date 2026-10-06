package qbittorrent

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func TestQBBanBatchesResumeAndSubmitOnlyChanges(t *testing.T) {
	var mu sync.Mutex
	requests, endpoints := 0, 0
	succeeded := make(map[string]bool)
	c, settings := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	settings.NewBanPeersMethod = true
	peers := map[string]client.BanTarget{"192.0.2.20": {Ports: map[int]bool{-1: true}}}
	if c.SubmitBlockPeer(peers) {
		t.Fatal("failed batch was reported as successful")
	}
	if !c.SubmitBlockPeer(peers) {
		t.Fatal("retry did not finish remaining batches")
	}
	mu.Lock()
	count, total := requests, endpoints
	mu.Unlock()
	if total != 2*65536 {
		t.Fatalf("acknowledged endpoints=%d", total)
	}
	if !c.SubmitBlockPeer(peers) {
		t.Fatal("unchanged list failed")
	}
	mu.Lock()
	unchanged := requests
	mu.Unlock()
	if unchanged != count {
		t.Fatal("unchanged list generated a request")
	}
	peers["198.51.100.1"] = client.BanTarget{Ports: map[int]bool{6881: true}}
	if !c.SubmitBlockPeer(peers) {
		t.Fatal("new IP failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != count+1 || !succeeded["198.51.100.1:6881"] {
		t.Fatal("new IP was not submitted independently")
	}
}

func TestQBLegacyBanRetainsReplacementSemantics(t *testing.T) {
	var lists []string
	c, settings := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	settings.NewBanPeersMethod = false
	peers := map[string]client.BanTarget{"192.0.2.1": {Ports: map[int]bool{1: true}}}
	if !c.SubmitBlockPeer(peers) {
		t.Fatal("first submission failed")
	}
	peers["192.0.2.1"].Ports[2] = true
	if !c.SubmitBlockPeer(peers) || len(lists) != 1 {
		t.Fatal("IP-only API resent a port-only change")
	}
	peers["198.51.100.1"] = client.BanTarget{}
	if !c.SubmitBlockPeer(peers) || len(lists) != 2 || !strings.Contains(lists[1], "192.0.2.1\n") || !strings.Contains(lists[1], "198.51.100.1\n") {
		t.Fatal("replacement omitted existing IPs")
	}
	delete(peers, "192.0.2.1")
	if !c.SubmitBlockPeer(peers) || strings.Contains(lists[2], "192.0.2.1") {
		t.Fatal("replacement retained removed IP")
	}
}
