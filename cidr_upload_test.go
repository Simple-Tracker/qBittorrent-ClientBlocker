package main

import "testing"

func TestCIDRUploadSumsObservedIncrements(t *testing.T) {
	for _, tc := range cidrTestCases {
		for _, newIPs := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/existing", true: "/new"}[newIPs], func(t *testing.T) {
				installCIDRTest(t, tc.mask4, tc.mask6)
				UpdateConfig(func(c *ConfigStruct) {
					c.MaxIPPortCount = 0
					c.IPUpCheckIncrementMB = 100
					c.IPUpCheckPerTorrentRatio = 0
				})
				submitted := installCIDRBanServer(t)
				ips := []string{tc.ip, tc.neighbor, tc.outside}
				if newIPs {
					CheckAllIP(ipMap, lastIPMap)
					currentTimestamp += 2
				}
				for _, ip := range ips {
					processCIDRTestPeer(ip, 6881, .5, 1000<<20)
				}
				if !newIPs {
					if n := CheckAllIP(ipMap, lastIPMap); n != 0 {
						t.Fatalf("initial raw counters triggered %d bans", n)
					}
					currentTimestamp += 2
				}
				for _, ip := range ips {
					processCIDRTestPeer(ip, 6881, .5, 1060<<20)
				}
				want := 0
				if tc.network != "" {
					want = 2
				}
				if n := CheckAllIP(ipMap, lastIPMap); n != want {
					t.Fatalf("60+60 MiB bans=%d, want %d", n, want)
				}
				if _, banned := blockPeerMap[tc.outside]; banned {
					t.Fatal("upload from a different network was included")
				}
				if want > 0 {
					if !QB_SubmitBlockPeer(blockPeerMap) {
						t.Fatal("ban submission failed")
					}
					assertCIDRTestIPSet(t, *submitted, expectedCIDRBanIPs(tc.ip, tc.neighbor))
				}
			})
		}
	}
}

func TestCIDRUploadUsesCurrentMask(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	UpdateConfig(func(c *ConfigStruct) { c.MaxIPPortCount = 0; c.IPUpCheckIncrementMB = 100 })
	ips := []string{"203.0.113.10", "203.0.113.20"}
	for _, ip := range ips {
		processCIDRTestPeer(ip, 6881, .5, 100<<20)
	}
	CheckAllIP(ipMap, lastIPMap)
	currentTimestamp += 2
	processCIDRTestPeer(ips[0], 6881, .5, 160<<20)
	UpdateConfig(func(c *ConfigStruct) { c.BanIPCIDR = "/24" })
	processCIDRTestPeer(ips[1], 6881, .5, 160<<20)
	if n := CheckAllIP(ipMap, lastIPMap); n != 2 {
		t.Fatalf("mask reload split the current subnet: %d bans", n)
	}
}

func TestCIDRUploadHonorsDisabledCheck(t *testing.T) {
	installCIDRTest(t, "/24", "/128")
	UpdateConfig(func(c *ConfigStruct) { c.IPUploadedCheck = false; c.IPUpCheckIncrementMB = 1 })
	processCIDRTestPeer("203.0.113.10", 6881, .5, 1<<20)
	CheckAllIP(ipMap, lastIPMap)
	currentTimestamp += 2
	processCIDRTestPeer("203.0.113.10", 6881, .5, 100<<20)
	if n := CheckAllIP(ipMap, lastIPMap); n != 0 {
		t.Fatalf("disabled upload check triggered %d bans", n)
	}
}
