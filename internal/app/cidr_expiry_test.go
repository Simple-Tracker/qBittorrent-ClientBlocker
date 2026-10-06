package app

import "testing"

func TestClearBlockPeerCIDRReloadDoesNotReviveExpiredSubnet(t *testing.T) {
	installCIDRTest(t, "/24", "/128")
	AddBlockPeer("test", "initial", "203.0.113.10", 6881, "hash", "id", "client", 0, 1)
	UpdateConfig(func(c *ConfigStruct) { c.BanIPCIDR = "/32" })
	currentTimestamp = 111
	if count := ClearBlockPeer(); count != 1 {
		t.Fatalf("expired peers removed=%d, want 1", count)
	}
	if len(blockPeerMap) != 0 || len(blockCIDRMap) != 0 {
		t.Fatalf("expired bans retained after mask change: peers=%v CIDRs=%v", blockPeerMap, blockCIDRMap)
	}
	UpdateConfig(func(c *ConfigStruct) { c.BanIPCIDR = "/24" })
	currentTimestamp = 200
	if count := processCIDRTestPeer("203.0.113.20", 6882, .5, 1); count != 0 {
		t.Fatal("restoring the mask revived the expired subnet ban")
	}
}

func TestClearBlockPeerUsesStoredCIDRRenewal(t *testing.T) {
	for _, mask := range []string{"/24", "/32"} {
		t.Run(mask, func(t *testing.T) {
			installCIDRTest(t, "/24", "/128")
			AddBlockPeer("test", "initial", "203.0.113.10", 6881, "hash", "id", "client", 0, 1)
			currentTimestamp = 105
			AddBlockPeer("test", "neighbor", "203.0.113.20", 6882, "hash", "id", "client", 0, 1)
			UpdateConfig(func(c *ConfigStruct) { c.BanIPCIDR = mask })
			currentTimestamp = 111
			if count := ClearBlockPeer(); count != 0 || blockPeerMap["203.0.113.10"].Timestamp != 105 {
				t.Fatalf("stored subnet activity did not renew the earlier ban: removed=%d peers=%v", count, blockPeerMap)
			}
			currentTimestamp = 116
			if count := ClearBlockPeer(); count != 2 || len(blockPeerMap) != 0 || len(blockCIDRMap) != 0 {
				t.Fatalf("expired bans retained: removed=%d peers=%v CIDRs=%v", count, blockPeerMap, blockCIDRMap)
			}
		})
	}
}
