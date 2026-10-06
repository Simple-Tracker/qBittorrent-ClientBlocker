package qbittorrent

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func TestQBRejectsInvalidBanIPsBeforeRequestsOrCacheChanges(t *testing.T) {
	for _, mode := range []string{"preferences", "banPeers", "shadowBan"} {
		t.Run(mode, func(t *testing.T) {
			requests := 0
			c, settings := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Write([]byte("Ok."))
			}))
			settings.NewBanPeersMethod = mode == "banPeers"
			submit := c.SubmitBlockPeer
			if mode == "shadowBan" {
				submit = c.SubmitShadowBanPeer
			}
			peers := map[string]client.BanTarget{"192.0.2.1": {Ports: map[int]bool{6881: true}}}
			if !submit(peers) || requests != 1 {
				t.Fatal("valid initial submission failed")
			}
			cacheBefore, _ := json.Marshal(c.banCache)
			peers["192.0.2.2"] = client.BanTarget{Ports: map[int]bool{-1: true}}
			for _, invalid := range []string{"203.0.113.0/24", "203.0.113.10/32", "2001:db8::/60", "::ffff:203.0.113.0/24", "", "invalid", "203.0.113.10:6881"} {
				peers[invalid] = client.BanTarget{Ports: map[int]bool{6881: true}}
				if submit(peers) || requests != 1 {
					t.Fatalf("invalid address %q caused a request or was reported successful", invalid)
				}
				cacheAfter, _ := json.Marshal(c.banCache)
				if string(cacheAfter) != string(cacheBefore) || len(c.banBatches) != 0 {
					t.Fatalf("invalid address %q changed submission cache", invalid)
				}
				// 直接调用旧版辅助方法时也必须执行相同的校验.
				if mode != "shadowBan" && (c.submitBlockPeer(peers) || requests != 1) {
					t.Fatalf("direct submission accepted %q", invalid)
				}
				delete(peers, invalid)
			}
			peers["192.0.2.2"] = client.BanTarget{Ports: map[int]bool{6882: true}}
			if !submit(peers) || requests != 2 {
				t.Fatal("valid list did not recover after invalid input was removed")
			}
			if mode != "shadowBan" && (!submit(peers) || requests != 2) {
				t.Fatal("successful list was not cached")
			}
		})
	}
}

func TestQBValidatesCachedBanIPs(t *testing.T) {
	for _, newMethod := range []bool{false, true} {
		c := New(client.Services{Snapshot: func() client.Settings { return client.Settings{NewBanPeersMethod: newMethod} }})
		peers := map[string]client.BanTarget{"203.0.113.0/24": {Ports: map[int]bool{6881: true}}}
		c.banCache = map[string]map[int]bool{"203.0.113.0/24": {6881: true}}
		c.banMethod = newMethod
		if c.SubmitBlockPeer(peers) {
			t.Fatal("cache hit bypassed address validation")
		}
	}
}
