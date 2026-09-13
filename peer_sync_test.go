package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQBPeerSyncSharesCursorAndMergesFields(t *testing.T) {
	oldCfg, oldClient, oldNow := ConfigSnapshot(), httpClient, currentTimestamp
	t.Cleanup(func() { ReplaceConfig(oldCfg); httpClient, currentTimestamp = oldClient, oldNow })
	responses := []string{
		`{"rid":1,"full_update":true,"peers":{"peer":{"ip":"192.0.2.1","port":6881,"client":"original","uploaded":10}}}`,
		`{"rid":2,"peers":{"peer":{"uploaded":0,"client":"changed"}}}`,
		`{"rid":3,"peers_removed":["peer"]}`,
		`{"rid":4,"peers":{"broken":{"port":"invalid"}}}`,
		`{"rid":1,"full_update":true,"peers":{"new":{"ip":"198.51.100.1","port":6882}}}`,
		`{"rid":1,"full_update":true,"peers":{}}`,
	}
	wantRID := []string{"0", "1", "2", "3", "0", "0"}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls >= len(responses) {
			t.Error("unexpected request")
			http.Error(w, "extra", 500)
			return
		}
		if got := r.URL.Query().Get("rid"); got != wantRID[calls] {
			t.Errorf("call %d rid=%s, want %s", calls, got, wantRID[calls])
		}
		w.Write([]byte(responses[calls]))
		calls++
	}))
	defer server.Close()
	cfg := *oldCfg
	cfg.ClientURL = server.URL
	ReplaceConfig(&cfg)
	httpClient = *server.Client()
	currentTimestamp = 100
	client := &QBClient{}
	first, _ := client.FetchTorrentPeers(&Torrent{Hash: "a"})
	second, _ := client.FetchTorrentPeers(&Torrent{Hash: "b"})
	if len(second) != 1 || second[0].IP != "192.0.2.1" || second[0].Port != 6881 || second[0].Uploaded != 0 || second[0].Client != "changed" {
		t.Fatalf("partial merge=%#v", second)
	}
	if first[0].Uploaded != 10 || first[0].Client != "original" {
		t.Fatal("previous caller snapshot was mutated")
	}
	if peers, _ := client.FetchTorrentPeers(&Torrent{Hash: "a"}); len(peers) != 0 {
		t.Fatal("removed peer retained")
	}
	if peers, _ := client.FetchTorrentPeers(&Torrent{Hash: "a"}); peers != nil {
		t.Fatal("malformed delta accepted")
	}
	if peers, _ := client.FetchTorrentPeers(&Torrent{Hash: "a"}); len(peers) != 1 || peers[0].IP != "198.51.100.1" {
		t.Fatal("failed sync did not recover")
	}
	currentTimestamp += 300
	if peers, _ := client.FetchTorrentPeers(&Torrent{Hash: "a"}); len(peers) != 0 {
		t.Fatal("periodic full reset retained stale peers")
	}
}

func TestRequestAndScanDelayCanBeCancelled(t *testing.T) {
	oldContext, oldClient := requestContext, httpClient
	ctx, cancel := context.WithCancel(context.Background())
	requestContext = ctx
	t.Cleanup(func() { cancel(); requestContext, httpClient = oldContext, oldClient })
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	httpClient = *server.Client()
	done := make(chan struct{})
	go func() { defer close(done); Fetch(server.URL, false, true, false, nil) }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not cancel")
	}
	if WaitRequestDelay(time.Minute) {
		t.Fatal("cancelled scan delay completed normally")
	}
}
