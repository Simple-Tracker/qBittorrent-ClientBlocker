package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestQBSubmitBlockPeerPreservesPayloads(t *testing.T) {
	oldConfig := *config
	oldHTTPClient := httpClient
	oldMethod := qB_useNewBanPeersMethod
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		httpClient = oldHTTPClient
		qB_useNewBanPeersMethod = oldMethod
	})

	var requestForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse request form: %v", err)
		}
		requestForm = r.PostForm
		_, _ = w.Write([]byte("Ok."))
	}))
	defer server.Close()
	httpClient = *server.Client()
	testConfig := oldConfig
	testConfig.ClientURL = server.URL
	testConfig.BanAllPort = false
	config = &testConfig

	peers := map[string]BlockPeerInfoStruct{
		"192.0.2.10": {Port: map[int]bool{6881: true}},
	}

	qB_useNewBanPeersMethod = true
	if !qB_SubmitBlockPeer(peers) {
		t.Fatal("new qBittorrent ban submission failed")
	}
	if got := requestForm.Get("peers"); got != "192.0.2.10:6881" {
		t.Fatalf("new API peers=%q", got)
	}

	allPortsPeer := map[string]BlockPeerInfoStruct{
		"192.0.2.20": {Port: map[int]bool{-1: true}},
	}
	if !qB_SubmitBlockPeer(allPortsPeer) {
		t.Fatal("all-port qBittorrent ban submission failed")
	}
	allPorts := strings.Split(requestForm.Get("peers"), "|")
	if len(allPorts) != 2*65536 {
		t.Fatalf("all-port API peer count=%d, want %d", len(allPorts), 2*65536)
	}
	if allPorts[0] != "192.0.2.20:0" || allPorts[1] != "[::ffff:192.0.2.20]:0" ||
		allPorts[len(allPorts)-2] != "192.0.2.20:65535" || allPorts[len(allPorts)-1] != "[::ffff:192.0.2.20]:65535" {
		t.Fatalf("all-port API boundaries=%q ... %q", allPorts[:2], allPorts[len(allPorts)-2:])
	}

	ipv6Peer := map[string]BlockPeerInfoStruct{
		"2001:db8::1": {Port: map[int]bool{6881: true}},
	}
	if !qB_SubmitBlockPeer(ipv6Peer) {
		t.Fatal("IPv6 qBittorrent ban submission failed")
	}
	if got := requestForm.Get("peers"); got != "[2001:db8::1]:6881" {
		t.Fatalf("IPv6 API peers=%q", got)
	}

	qB_useNewBanPeersMethod = false
	if !qB_SubmitBlockPeer(peers) {
		t.Fatal("legacy qBittorrent ban submission failed")
	}
	preference := requestForm.Get("json")
	for _, entry := range []string{"192.0.2.10\n", "::ffff:192.0.2.10\n"} {
		if !strings.Contains(preference, entry) {
			t.Fatalf("legacy preference %q does not contain %q", preference, entry)
		}
	}
}
