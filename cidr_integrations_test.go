package main

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCIDRBTNReportsKeepIndividualPeersAndHistory(t *testing.T) {
	installCIDRTest(t, "/24", "/60")
	oldGetting := btn_isGettingConfig.Load()
	t.Cleanup(func() { btn_isGettingConfig.Store(oldGetting) })
	btn_isGettingConfig.Store(false)
	var peers BTN_SubmitPeersStruct
	var histories BTN_SubmitHistoriesStruct
	var bans BTN_SubmitBansStruct
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		defer reader.Close()
		var target any
		switch r.URL.Path {
		case "/peers":
			target = &peers
		case "/histories":
			target = &histories
		case "/bans":
			target = &bans
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			return
		}
		if err := json.NewDecoder(reader).Decode(target); err != nil {
			t.Error(err)
		}
		w.Write([]byte("Ok."))
	}))
	ips := []string{cidrTestCases[0].ip, cidrTestCases[0].neighbor, cidrTestCases[1].ip, cidrTestCases[1].neighbor}
	for i, ip := range ips {
		processCIDRTestPeer(ip, 6881+i, .1, 1<<20)
	}
	DeepCopyTorrentMap(torrentMap, lastTorrentMap)
	currentTimestamp += 2
	// ProcessPeer must preserve the addresses consumed by BTN's serializers.
	for i := len(ips) - 1; i >= 0; i-- {
		processCIDRTestPeer(ips[i], 6881+i, .2, int64(3+i)<<20)
	}
	url := ConfigSnapshot().ClientURL
	btnStateMutex.Lock()
	btnConfig = &BTN_ConfigStruct{Ability: map[string]BTN_Ability{
		"submit_peers": {Endpoint: url + "/peers"}, "submit_histories": {Endpoint: url + "/histories"}, "submit_bans": {Endpoint: url + "/bans"},
	}}
	btnStateMutex.Unlock()
	BTN_SubmitPeers(torrentMap, currentTimestamp)
	BTN_SubmitHistories(torrentMap, lastTorrentMap, currentTimestamp)
	if len(peers.Peers) != len(ips) || len(histories.Peers) != len(ips) {
		t.Fatalf("report sizes: peers=%d histories=%d", len(peers.Peers), len(histories.Peers))
	}
	var peerIPs, historyIPs []string
	for _, peer := range peers.Peers {
		peerIPs = append(peerIPs, peer.IPAddress)
		if peer.PeerProgress != .2 || peer.PeerID != peer.IPAddress {
			t.Fatalf("mixed peer report: %v", peer)
		}
	}
	for _, peer := range histories.Peers {
		historyIPs = append(historyIPs, peer.IPAddress)
		index := peer.PeerPort - 6881
		if index < 0 || index >= len(ips) || peer.IPAddress != ips[index] || peer.UploadedOffset != int64(2+index)<<20 || peer.DownloadedOffset != int64(2+index)<<19 {
			t.Fatalf("mixed history report: %v", peer)
		}
	}
	assertCIDRTestIPSet(t, peerIPs, ips)
	assertCIDRTestIPSet(t, historyIPs, ips)
	for _, ip := range ips {
		AddBlockPeer("test", "report", ip, 6881, "hash", ip, "test", 1, 2)
	}
	BTN_SubmitBans(blockPeerMap, currentTimestamp)
	var banIPs []string
	for _, ban := range bans.Bans {
		banIPs = append(banIPs, ban.Peer.IPAddress)
	}
	assertCIDRTestIPSet(t, banIPs, ips)
}

func TestCIDRBansOnOtherClients(t *testing.T) {
	for _, clientType := range []string{"Transmission", "BitComet"} {
		t.Run(clientType, func(t *testing.T) {
			installCIDRTest(t, "/24", "/60")
			oldFilter := Tr_ipfilterStr
			t.Cleanup(func() { Tr_ipfilterStr = oldFilter })
			var submitted []string
			requests := 0
			InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if clientType == "BitComet" {
					var body BC_v2_BanParams
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.TaskID != "hash" || body.BanTime != "ban_ip_forever" {
						t.Errorf("BitComet request changed: %v", body)
					}
					submitted = body.IPList
				}
				w.Write([]byte(`{"result":"success"}`))
			}))
			currentClientType = clientType
			UpdateConfig(func(c *ConfigStruct) { c.PortBlockList = []uint32{6881} })
			ips := []string{cidrTestCases[0].ip, cidrTestCases[1].ip}
			for _, ip := range ips {
				if processCIDRTestPeer(ip, 6881, .5, 1<<20) != 1 {
					t.Fatal("peer was not banned")
				}
			}
			var client Client = &BCClient{Version: 2}
			if clientType == "Transmission" {
				client = &TRClient{}
			}
			if !client.SubmitBlockPeer(blockPeerMap) || requests != 1 {
				t.Fatal("client ban submission failed")
			}
			if clientType == "Transmission" {
				submitted = strings.Fields(Tr_ipfilterStr)
			}
			assertCIDRTestIPSet(t, submitted, ips)
		})
	}
}
