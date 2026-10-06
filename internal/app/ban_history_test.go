package app

import "testing"

func TestOfflineHistoryDoesNotRenewOrRecreateBan(t *testing.T) {
	for _, mask := range []string{"/32", "/24"} {
		t.Run(mask, func(t *testing.T) {
			installCIDRTest(t, mask, "/128")
			UpdateConfig(func(c *ConfigStruct) { c.HistoryRetention = 3600 })
			ip := "203.0.113.10"
			processCIDRTestPeer(ip, 6881, .5, 100)
			processCIDRTestPeer(ip, 6882, .5, 100)
			if n := statistics.CheckAllIP(); n != 1 {
				t.Fatalf("initial statistical bans=%d", n)
			}
			for currentTimestamp = 102; currentTimestamp <= 130; currentTimestamp += 2 {
				ClearBlockPeer()
				statistics.CheckAllIP()
				statistics.CheckAllTorrent()
			}
			if _, exists := blockPeerMap[ip]; exists {
				t.Fatal("offline historical records renewed or recreated the expired ban")
			}
		})
	}
}

func TestObservedBlockedPeerStillRenewsBan(t *testing.T) {
	installCIDRTest(t, "/24", "/128")
	ip := "203.0.113.10"
	AddBlockPeer("test", "test", ip, 6881, "hash", "id", "client", 0, 100)
	currentTimestamp = 105
	processCIDRTestPeer(ip, 6881, .5, 100)
	currentTimestamp = 111
	ClearBlockPeer()
	if peer, exists := blockPeerMap[ip]; !exists || peer.Timestamp != 105 {
		t.Fatal("a real observation did not renew the ban")
	}
}
