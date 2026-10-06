package main

import (
	"encoding/json"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
)

var cidrTestCases = []struct {
	name, mask4, mask6, ip, neighbor, outside, network string
}{
	{"IPv4Subnet", "/24", "/128", "203.0.113.10", "203.0.113.20", "203.0.114.10", "203.0.113.0/24"},
	{"IPv6Subnet", "/32", "/60", "2001:db8:1234:7a51::10", "2001:db8:1234:7a5f::20", "2001:db8:1234:7a60::10", "2001:db8:1234:7a50::/60"},
	{"IPv4Host", "/32", "/128", "203.0.113.10", "203.0.113.20", "203.0.114.10", ""},
	{"IPv6Host", "/32", "/128", "2001:db8:1234:7a51::10", "2001:db8:1234:7a5f::20", "2001:db8:1234:7a60::10", ""},
}

func installCIDRTest(t *testing.T, mask4, mask6 string) {
	t.Helper()
	InstallHistoryTest(t)
	oldPeers, oldCIDRs := blockPeerMap, blockCIDRMap
	oldIPClean, oldTorrentClean, oldClean := lastIPCleanTimestamp, lastTorrentCleanTimestamp, lastCleanTimestamp
	oldClient, oldMethod := currentClientType, qB_useNewBanPeersMethod
	oldRules := syncServer_CompiledRules
	oldBTN, _, _ := BtnSnapshot()
	t.Cleanup(func() {
		blockPeerMap, blockCIDRMap = oldPeers, oldCIDRs
		lastIPCleanTimestamp, lastTorrentCleanTimestamp, lastCleanTimestamp = oldIPClean, oldTorrentClean, oldClean
		currentClientType, qB_useNewBanPeersMethod = oldClient, oldMethod
		syncServer_CompiledRules = oldRules
		btnStateMutex.Lock()
		btnConfig = oldBTN
		btnStateMutex.Unlock()
	})
	for _, rules := range []*sync.Map{&blockListCompiled, &ipBlockListCompiled} {
		rules := rules
		saved := make(map[any]any)
		rules.Range(func(k, v any) bool { saved[k] = v; return true })
		EraseSyncMap(rules)
		t.Cleanup(func() {
			EraseSyncMap(rules)
			for k, v := range saved {
				rules.Store(k, v)
			}
		})
	}
	ReplaceConfig(&ConfigStruct{
		BanIPCIDR: mask4, BanIP6CIDR: mask6, BanTime: 10,
		IPUpCheckInterval: 1, TorrentMapCleanInterval: 1,
		MaxIPPortCount: 1, IPUploadedCheck: true,
		IPUpCheckIncrementMB: 1000, IPUpCheckPerTorrentRatio: 1000,
	})
	blockPeerMap, blockCIDRMap = make(map[string]BlockPeerInfoStruct), make(map[string]BlockCIDRInfoStruct)
	lastIPCleanTimestamp, lastTorrentCleanTimestamp, lastCleanTimestamp = 0, 0, 0
	currentClientType, qB_useNewBanPeersMethod = "qBittorrent", false
	syncServer_CompiledRules = nil
	btnStateMutex.Lock()
	btnConfig = nil
	btnStateMutex.Unlock()
}

func processCIDRTestPeer(ip string, port int, progress float64, uploaded int64) int {
	var blocks, ipBlocks, bad, empty int
	ProcessPeer(&Peer{IP: ip, Port: port, ID: ip, Client: "test", DlSpeed: 1,
		Progress: progress, Uploaded: uploaded, Downloaded: uploaded / 2},
		"hash", 100<<20, &blocks, &ipBlocks, &bad, &empty)
	return blocks + ipBlocks
}

func checkCIDRTestStatistics() int {
	ips := CheckAllIP(ipMap, lastIPMap)
	peers, torrentIPs := CheckAllTorrent(torrentMap, lastTorrentMap)
	return ips + peers + torrentIPs
}

func assertCIDRTestIPSet(t *testing.T, actual, expected []string) {
	t.Helper()
	got, want := make(map[string]bool), make(map[string]bool)
	for _, ip := range actual {
		if net.ParseIP(ip) == nil {
			t.Fatalf("submitted non-IP address %q", ip)
		}
		got[ip] = true
	}
	for _, ip := range expected {
		want[ip] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IP set=%v, want %v", got, want)
	}
}

// Capture and validate the actual setPreferences body, including mapped IPv4 addresses.
func installCIDRBanServer(t *testing.T) *[]string {
	t.Helper()
	var submitted []string
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/app/setPreferences" {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		var prefs map[string]string
		if err := json.Unmarshal([]byte(r.Form.Get("json")), &prefs); err != nil {
			t.Error(err)
		}
		submitted = strings.Fields(prefs["banned_IPs"])
		w.Write([]byte("Ok."))
	}))
	return &submitted
}

func expectedCIDRBanIPs(ips ...string) []string {
	result := append([]string(nil), ips...)
	for _, ip := range ips {
		if net.ParseIP(ip).To4() != nil {
			result = append(result, "::ffff:"+ip)
		}
	}
	return result
}

func TestCIDRStatisticsKeepPeerIPsIndependent(t *testing.T) {
	for _, tc := range cidrTestCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				name := "forward"
				if reverse {
					name = "reverse"
				}
				t.Run(name, func(t *testing.T) {
					installCIDRTest(t, tc.mask4, tc.mask6)
					ips := []string{tc.ip, tc.neighbor}
					if reverse {
						ips[0], ips[1] = ips[1], ips[0]
					}
					for i, ip := range ips {
						processCIDRTestPeer(ip, 6881+i, .2+float64(i)*.3, int64(10+i*10)<<20)
					}
					if len(ipMap) != 2 || len(torrentMap["hash"].Peers) != 2 {
						t.Fatalf("peers merged: IPs=%v torrent=%v", ipMap, torrentMap)
					}
					if count := checkCIDRTestStatistics(); count != 0 {
						t.Fatalf("independent peers triggered %d bans", count)
					}
					for i, ip := range ips {
						info := ipMap[ip]
						peer := torrentMap["hash"].Peers[ip]
						if len(info.Port) != 1 || !info.Port[6881+i] || peer.ID != ip || peer.Uploaded != int64(10+i*10)<<20 {
							t.Fatalf("unexpected statistics for %s: %v / %v", ip, info, peer)
						}
						if tc.network == "" {
							if info.Net != nil || peer.Net != nil {
								t.Fatal("default masks should not create subnet metadata")
							}
						} else if info.Net == nil || info.Net.String() != tc.network || peer.Net == nil || peer.Net.String() != tc.network {
							t.Fatal("subnet metadata was lost")
						}
					}

					currentTimestamp += 2
					// Reverse the observation order and give each IP a different delta.
					processCIDRTestPeer(ips[1], 6882, .6, 24<<20)
					processCIDRTestPeer(ips[0], 6881, .3, 12<<20)
					for i, ip := range ips {
						previous := int64(10+i*10) << 20
						if lastIPMap[ip].TorrentUploaded["hash"] != previous || lastTorrentMap["hash"].Peers[ip].Uploaded != previous {
							t.Fatal("observations overwrote an independent history snapshot")
						}
						peer := torrentMap["hash"].Peers[ip]
						if peer.Uploaded != int64(12+i*12)<<20 || peer.Downloaded != int64(6+i*6)<<20 || peer.Progress != .3+float64(i)*.3 {
							t.Fatalf("current counters crossed IPs: %v", peer)
						}
					}
					if count := checkCIDRTestStatistics(); count != 0 {
						t.Fatalf("second cycle triggered %d bans", count)
					}
					// Multiple ports on the same IP keep independent counters.
					processCIDRTestPeer(ips[0], 6883, .4, 14<<20)
					peer := torrentMap["hash"].Peers[ips[0]]
					if len(peer.Port) != 2 || peer.Progress != .4 || peer.Uploaded != 26<<20 || len(ipMap[ips[1]].Port) != 1 {
						t.Fatalf("same-IP port behavior changed: %v", peer)
					}
					ok, payload := SyncWithServer_PrepareJSON(torrentMap)
					var submitted SyncServer_SubmitStruct
					if !ok || json.Unmarshal([]byte(payload), &submitted) != nil {
						t.Fatal("cannot serialize independent peers")
					}
					var addresses []string
					for ip := range submitted.TorrentMap["hash"].Peers {
						addresses = append(addresses, ip)
					}
					assertCIDRTestIPSet(t, addresses, ips)
				})
			}
		})
	}
}

func TestCIDRStatisticalBansSubmitActualIPs(t *testing.T) {
	for _, tc := range cidrTestCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, rule := range []string{"ports", "globalUpload", "torrentUpload", "relativeProgress"} {
				t.Run(rule, func(t *testing.T) {
					installCIDRTest(t, tc.mask4, tc.mask6)
					submitted := installCIDRBanServer(t)
					UpdateConfig(func(c *ConfigStruct) {
						c.MaxIPPortCount = 0
						c.IPUpCheckPerTorrentRatio = 0
						switch rule {
						case "ports":
							c.MaxIPPortCount = 1
							c.IPUploadedCheck = false
						case "globalUpload":
							c.IPUpCheckIncrementMB = 1
						case "torrentUpload":
							c.IPUpCheckPerTorrentRatio = 3
						case "relativeProgress":
							c.BanByRelativeProgressUploaded = true
							c.BanByRelativePUStartMB, c.BanByRelativePUStartPercent, c.BanByRelativePUAntiErrorRatio = 1, 3, 3
						}
					})
					processCIDRTestPeer(tc.ip, 6881, .1, 1<<20)
					if count := checkCIDRTestStatistics(); count != 0 {
						t.Fatalf("cold start triggered %d bans", count)
					}
					currentTimestamp += 2
					port, progress, uploaded := 6881, .11, int64(10<<20)
					if rule == "ports" {
						port = 6882
					} else if rule == "torrentUpload" {
						uploaded = 40 << 20
					}
					processCIDRTestPeer(tc.ip, port, progress, uploaded)
					// A newly observed neighbor must establish its own upload baseline.
					processCIDRTestPeer(tc.neighbor, 6883, .9, 20<<20)
					if count := checkCIDRTestStatistics(); count != 1 {
						t.Fatalf("statistical bans=%d, want 1", count)
					}
					if len(blockPeerMap) != 1 || blockPeerMap[tc.ip].Module == "" {
						t.Fatalf("wrong ban identity: %v", blockPeerMap)
					}
					if tc.network != "" && !blockCIDRMap[tc.network].IPs[tc.ip] {
						t.Fatalf("missing subnet membership: %v", blockCIDRMap)
					}
					if !QB_SubmitBlockPeer(blockPeerMap) {
						t.Fatal("ban submission failed")
					}
					assertCIDRTestIPSet(t, *submitted, expectedCIDRBanIPs(tc.ip))
				})
			}
		})
	}
}

func TestCIDRFixedRulesAndRuntimeBanLifecycle(t *testing.T) {
	for _, tc := range cidrTestCases[:2] {
		t.Run(tc.name, func(t *testing.T) {
			installCIDRTest(t, tc.mask4, tc.mask6)
			submitted := installCIDRBanServer(t)
			ipBlockListCompiled.Store(tc.network, ParseIPCIDR(tc.network))
			if count := processCIDRTestPeer(tc.ip, 6881, .5, 1<<20); count != 1 || blockPeerMap[tc.ip].Reason != "Bad-IP_Normal" {
				t.Fatal("fixed CIDR rule did not ban the observed peer")
			}
			client := &QBClient{}
			if !client.SubmitBlockPeer(blockPeerMap) {
				t.Fatal("initial submission failed")
			}
			assertCIDRTestIPSet(t, *submitted, expectedCIDRBanIPs(tc.ip))
			ipBlockListCompiled.Delete(tc.network)
			currentTimestamp = 105
			if count := processCIDRTestPeer(tc.neighbor, 6882, .5, 1<<20); count != 1 || blockPeerMap[tc.neighbor].Reason != "Bad-CIDR" {
				t.Fatal("runtime subnet did not ban the later peer")
			}
			if count := processCIDRTestPeer(tc.outside, 6883, .5, 1<<20); count != 0 {
				t.Fatal("subnet ban escaped its range")
			}
			if !client.SubmitBlockPeer(blockPeerMap) {
				t.Fatal("expanded submission failed")
			}
			assertCIDRTestIPSet(t, *submitted, expectedCIDRBanIPs(tc.ip, tc.neighbor))
			members := blockCIDRMap[tc.network].IPs
			if len(members) != 2 || !members[tc.ip] || !members[tc.neighbor] {
				t.Fatalf("unexpected subnet membership: %v", members)
			}
			currentTimestamp = 111
			if count := ClearBlockPeer(); count != 0 || blockPeerMap[tc.ip].Timestamp != 105 {
				t.Fatal("newer subnet activity did not renew the earlier ban")
			}
			currentTimestamp = 116
			if count := ClearBlockPeer(); count != 2 || len(blockPeerMap) != 0 || len(blockCIDRMap) != 0 {
				t.Fatalf("expired bans not cleared: peers=%v CIDRs=%v", blockPeerMap, blockCIDRMap)
			}
			if !client.SubmitBlockPeer(blockPeerMap) {
				t.Fatal("empty replacement submission failed")
			}
			assertCIDRTestIPSet(t, *submitted, nil)
			if count := processCIDRTestPeer(tc.neighbor, 6882, .5, 1<<20); count != 0 {
				t.Fatal("expired subnet continued banning peers")
			}
		})
	}
}

func TestAddBlockPeerRejectsInvalidIPs(t *testing.T) {
	installCIDRTest(t, "/24", "/60")
	for _, ip := range []string{"203.0.113.0/24", "203.0.113.10/32", "2001:db8::/60", "::ffff:203.0.113.0/24", "", "invalid", "203.0.113.10:6881"} {
		AddBlockPeer("test", "invalid", ip, 6881, "hash", "id", "client", 1, 2)
	}
	if len(blockPeerMap) != 0 || len(blockCIDRMap) != 0 {
		t.Fatalf("invalid addresses reached ban state: peers=%v CIDRs=%v", blockPeerMap, blockCIDRMap)
	}
}

func TestCIDRProgressRollbackKeepsExistingDecision(t *testing.T) {
	for _, tc := range cidrTestCases[:2] {
		t.Run(tc.name, func(t *testing.T) {
			installCIDRTest(t, tc.mask4, tc.mask6)
			UpdateConfig(func(c *ConfigStruct) {
				c.BanByRelativeProgressUploaded = true
				c.BanByRelativePUStartMB, c.BanByRelativePUStartPercent, c.BanByRelativePUAntiErrorRatio = 1, 3, 3
			})
			processCIDRTestPeer(tc.ip, 6881, .5, 10<<20)
			checkCIDRTestStatistics()
			currentTimestamp += 2
			processCIDRTestPeer(tc.ip, 6881, .4, 20<<20)
			if count := checkCIDRTestStatistics(); count != 0 {
				t.Fatal("progress rollback alone changed the existing ban decision")
			}
		})
	}
}
