package qbittorrent

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func TestQBSubmitBlockPeerPreservesPayloads(t *testing.T) {
	var requestForm url.Values
	var peerForms []string
	c, settings := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse request form: %v", err)
		}
		requestForm = r.PostForm
		if r.PostForm.Has("peers") {
			peerForms = append(peerForms, r.PostForm.Get("peers"))
		}
		_, _ = w.Write([]byte("Ok."))
	}))

	peers := map[string]client.BanTarget{
		"192.0.2.10": {Ports: map[int]bool{6881: true}},
	}

	settings.NewBanPeersMethod = true
	if !c.submitBlockPeer(peers) {
		t.Fatal("new qBittorrent ban submission failed")
	}
	if got := requestForm.Get("peers"); got != "192.0.2.10:6881" {
		t.Fatalf("new API peers=%q", got)
	}

	allPortsPeer := map[string]client.BanTarget{
		"192.0.2.20": {Ports: map[int]bool{-1: true}},
	}
	peerForms = nil
	if !c.submitBlockPeer(allPortsPeer) {
		t.Fatal("all-port qBittorrent ban submission failed")
	}
	allPorts := strings.Split(strings.Join(peerForms, "|"), "|")
	if len(allPorts) != 2*65536 {
		t.Fatalf("all-port API peer count=%d, want %d", len(allPorts), 2*65536)
	}
	if allPorts[0] != "192.0.2.20:0" || allPorts[1] != "[::ffff:192.0.2.20]:0" ||
		allPorts[len(allPorts)-2] != "192.0.2.20:65535" || allPorts[len(allPorts)-1] != "[::ffff:192.0.2.20]:65535" {
		t.Fatalf("all-port API boundaries=%q ... %q", allPorts[:2], allPorts[len(allPorts)-2:])
	}

	ipv6Peer := map[string]client.BanTarget{
		"2001:db8::1": {Ports: map[int]bool{6881: true}},
	}
	if !c.submitBlockPeer(ipv6Peer) {
		t.Fatal("IPv6 qBittorrent ban submission failed")
	}
	if got := requestForm.Get("peers"); got != "[2001:db8::1]:6881" {
		t.Fatalf("IPv6 API peers=%q", got)
	}

	settings.NewBanPeersMethod = false
	if !c.submitBlockPeer(peers) {
		t.Fatal("legacy qBittorrent ban submission failed")
	}
	var preferences map[string]string
	if err := json.Unmarshal([]byte(requestForm.Get("json")), &preferences); err != nil {
		t.Fatal(err)
	}
	preference := preferences["banned_IPs"]
	for _, entry := range []string{"192.0.2.10\n", "::ffff:192.0.2.10\n"} {
		if !strings.Contains(preference, entry) {
			t.Fatalf("legacy preference %q does not contain %q", preference, entry)
		}
	}
}
