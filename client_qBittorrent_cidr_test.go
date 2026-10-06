package main

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestQBRejectsInvalidBanIPsBeforeRequestsOrCacheChanges(t *testing.T) {
	for _, mode := range []string{"preferences", "banPeers", "shadowBan"} {
		t.Run(mode, func(t *testing.T) {
			installCIDRTest(t, "/24", "/60")
			requests := 0
			InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Write([]byte("Ok."))
			}))
			qB_useNewBanPeersMethod = mode == "banPeers"
			client := &QBClient{}
			submit := client.SubmitBlockPeer
			if mode == "shadowBan" {
				submit = client.SubmitShadowBanPeer
			}
			peers := map[string]BlockPeerInfoStruct{"192.0.2.1": {Port: map[int]bool{6881: true}}}
			if !submit(peers) || requests != 1 {
				t.Fatal("valid initial submission failed")
			}
			cacheBefore, _ := json.Marshal(client.banCache)
			peers["192.0.2.2"] = BlockPeerInfoStruct{Port: map[int]bool{-1: true}}
			for _, invalid := range []string{"203.0.113.0/24", "203.0.113.10/32", "2001:db8::/60", "::ffff:203.0.113.0/24", "", "invalid", "203.0.113.10:6881"} {
				peers[invalid] = BlockPeerInfoStruct{Port: map[int]bool{6881: true}}
				if submit(peers) || requests != 1 {
					t.Fatalf("invalid address %q caused a request or was reported successful", invalid)
				}
				cacheAfter, _ := json.Marshal(client.banCache)
				if string(cacheAfter) != string(cacheBefore) || len(client.banBatches) != 0 {
					t.Fatalf("invalid address %q changed submission cache", invalid)
				}
				// Direct callers of the legacy helper must receive the same protection.
				if mode != "shadowBan" && (QB_SubmitBlockPeer(peers) || requests != 1) {
					t.Fatalf("direct submission accepted %q", invalid)
				}
				delete(peers, invalid)
			}
			peers["192.0.2.2"] = BlockPeerInfoStruct{Port: map[int]bool{6882: true}}
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
		installCIDRTest(t, "/24", "/60")
		qB_useNewBanPeersMethod = newMethod
		peers := map[string]BlockPeerInfoStruct{"203.0.113.0/24": {Port: map[int]bool{6881: true}}}
		client := &QBClient{
			banCache:  map[string]map[int]bool{"203.0.113.0/24": {6881: true}},
			banMethod: newMethod, banURL: ConfigSnapshot().ClientURL,
		}
		if client.SubmitBlockPeer(peers) {
			t.Fatal("cache hit bypassed address validation")
		}
	}
}

func TestCIDRBansSubmitValidQBEndpoints(t *testing.T) {
	for _, mode := range []string{"banPeers", "shadowBan"} {
		t.Run(mode, func(t *testing.T) {
			installCIDRTest(t, "/24", "/60")
			var addresses []string
			InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				for _, endpoint := range strings.Split(r.Form.Get("peers"), "|") {
					ip, _, err := net.SplitHostPort(endpoint)
					if err != nil {
						t.Errorf("invalid endpoint %q: %v", endpoint, err)
					}
					addresses = append(addresses, ip)
				}
				w.Write([]byte("Ok."))
			}))
			// Use a port rule to exercise ProcessPeer and both configured subnet masks.
			UpdateConfig(func(c *ConfigStruct) { c.PortBlockList = []uint32{6881} })
			ips := []string{"203.0.113.10", "2001:db8:1234:7a51::10"}
			for _, ip := range ips {
				if processCIDRTestPeer(ip, 6881, .5, 1<<20) != 1 {
					t.Fatal("port rule did not ban the peer")
				}
			}
			client := &QBClient{}
			qB_useNewBanPeersMethod = true
			if mode == "shadowBan" {
				if !client.SubmitShadowBanPeer(blockPeerMap) {
					t.Fatal("shadow ban failed")
				}
				ips = expectedCIDRBanIPs(ips...)
			} else if !client.SubmitBlockPeer(blockPeerMap) {
				t.Fatal("endpoint ban failed")
			}
			assertCIDRTestIPSet(t, addresses, ips)
		})
	}
}
