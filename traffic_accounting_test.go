package main

import "testing"

func TestPeerTrafficCountersSeparatePortsAndSamplingOrder(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	UpdateConfig(func(c *ConfigStruct) { c.MaxIPPortCount = 0; c.IPUpCheckIncrementMB = 10 })
	ip := "203.0.113.10"
	processCIDRTestPeer(ip, 6881, .5, 100<<20)
	processCIDRTestPeer(ip, 6882, .5, 20<<20)
	CheckAllIP(ipMap, lastIPMap)
	DeepCopyTorrentMap(torrentMap, lastTorrentMap)
	currentTimestamp += 2
	processCIDRTestPeer(ip, 6882, .5, 21<<20)
	processCIDRTestPeer(ip, 6881, .5, 101<<20)
	if delta := IPUploadedDelta(ipMap[ip], lastIPMap[ip]); delta != 2<<20 {
		t.Fatalf("delta=%d, want 2 MiB", delta)
	}
	if n := CheckAllIP(ipMap, lastIPMap); n != 0 {
		t.Fatalf("false bans=%d", n)
	}
	if got := torrentMap["hash"].Peers[ip].Uploaded; got != 122<<20 {
		t.Fatalf("total=%d, want 122 MiB", got)
	}
	if got := lastTorrentMap["hash"].Peers[ip].Connections[6881].Uploaded; got != 100<<20 {
		t.Fatal("connection baseline aliases current sample")
	}
}

func TestPeerTrafficRetainsObservedGrowthBeforeCounterReset(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	UpdateConfig(func(c *ConfigStruct) { c.MaxIPPortCount = 0 })
	ip := "203.0.113.10"
	processCIDRTestPeer(ip, 6881, .5, 100<<20)
	CheckAllIP(ipMap, lastIPMap)
	currentTimestamp++
	processCIDRTestPeer(ip, 6881, .5, 150<<20)
	processCIDRTestPeer(ip, 6881, .1, 10<<20)
	if delta := IPUploadedDelta(ipMap[ip], lastIPMap[ip]); delta != 60<<20 {
		t.Fatalf("delta=%d, want 60 MiB", delta)
	}
	peer := torrentMap["hash"].Peers[ip].Connections[6881]
	if peer.Uploaded != 160<<20 || peer.RawUploaded != 10<<20 {
		t.Fatalf("cumulative=%d raw=%d", peer.Uploaded, peer.RawUploaded)
	}
}

func TestNewPortEstablishesIndependentUploadBaseline(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	UpdateConfig(func(c *ConfigStruct) { c.MaxIPPortCount = 0; c.IPUpCheckIncrementMB = 1 })
	ip := "203.0.113.10"
	processCIDRTestPeer(ip, 6881, .5, 10<<20)
	CheckAllIP(ipMap, lastIPMap)
	currentTimestamp += 2
	processCIDRTestPeer(ip, 6882, .5, 1000<<20)
	if n := CheckAllIP(ipMap, lastIPMap); n != 0 {
		t.Fatalf("first connection sample caused %d bans", n)
	}
	currentTimestamp += 2
	processCIDRTestPeer(ip, 6882, .5, 1003<<20)
	if delta := IPUploadedDelta(ipMap[ip], lastIPMap[ip]); delta != 3<<20 {
		t.Fatalf("subsequent delta=%d", delta)
	}
}

func TestTorrentRulesUseMatchingConnectionProgress(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	UpdateConfig(func(c *ConfigStruct) { c.MaxIPPortCount = 0; c.IPUpCheckPerTorrentRatio = 2 })
	ip := "203.0.113.10"
	processCIDRTestPeer(ip, 6881, .8, 70<<20)
	processCIDRTestPeer(ip, 6882, .01, 1<<20)
	if peers, ips := CheckAllTorrent(torrentMap, lastTorrentMap); peers+ips != 0 {
		t.Fatal("mixed connection totals and progress triggered a ban")
	}
	currentTimestamp += 2
	processCIDRTestPeer(ip, 6881, .8, 71<<20)
	processCIDRTestPeer(ip, 6882, .01, 4<<20)
	if _, ips := CheckAllTorrent(torrentMap, lastTorrentMap); ips != 1 {
		t.Fatalf("per-connection violation bans=%d", ips)
	}
}

func TestUnknownTrafficCountersDoNotBecomeNegativeDeltas(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	ip := "203.0.113.10"
	for i := 0; i < 2; i++ {
		AddIPInfo(nil, ip, 6881, "hash", -1, -1)
		AddTorrentInfo("hash", 1000, nil, ip, 6881, .5, -1, -1, "id", "client")
	}
	if got := torrentMap["hash"].Peers[ip].Uploaded; got != -1 {
		t.Fatalf("unknown total=%d", got)
	}
	CheckAllIP(ipMap, lastIPMap)
	AddIPInfo(nil, ip, 6881, "hash", 100, 200)
	AddTorrentInfo("hash", 1000, nil, ip, 6881, .5, 100, 200, "id", "client")
	if got := torrentMap["hash"].Peers[ip].Uploaded; got != 200 {
		t.Fatalf("known total=%d", got)
	}
	if delta := IPUploadedDelta(ipMap[ip], lastIPMap[ip]); delta != 0 {
		t.Fatalf("unknown baseline produced delta=%d", delta)
	}
}

func TestUnknownSamplePreservesLastKnownCounter(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	ip := "203.0.113.10"
	for _, uploaded := range []int64{100, -1, 110} {
		AddIPInfo(nil, ip, 6881, "hash", uploaded, uploaded)
		AddTorrentInfo("hash", 1000, nil, ip, 6881, .5, uploaded, uploaded, "id", "client")
		if uploaded == 100 {
			DeepCopyIPMap(ipMap, lastIPMap)
		}
	}
	if got := ipMap[ip].TorrentUploaded["hash"]; got != 110 {
		t.Fatalf("IP total=%d", got)
	}
	if got := torrentMap["hash"].Peers[ip].Uploaded; got != 110 {
		t.Fatalf("torrent total=%d", got)
	}
	if delta := IPUploadedDelta(ipMap[ip], lastIPMap[ip]); delta != 10 {
		t.Fatalf("delta=%d", delta)
	}
}

func TestNewTorrentCountsOnlyGrowthObservedInsideWindow(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	ip := "203.0.113.10"
	AddIPInfo(nil, ip, 6881, "old", 0, 10)
	DeepCopyIPMap(ipMap, lastIPMap)
	AddIPInfo(nil, ip, 6881, "new", 0, 100)
	AddIPInfo(nil, ip, 6881, "new", 0, 160)
	if delta := IPUploadedDelta(ipMap[ip], lastIPMap[ip]); delta != 60 {
		t.Fatalf("delta=%d", delta)
	}
}

func TestRelativeProgressUsesCurrentSessionCounters(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	UpdateConfig(func(c *ConfigStruct) {
		c.BanByRelativeProgressUploaded = true
		c.BanByRelativePUStartMB = 1
		c.BanByRelativePUStartPercent = 3
		c.BanByRelativePUAntiErrorRatio = 3
	})
	ip := "203.0.113.10"
	processCIDRTestPeer(ip, 6881, .8, 1000<<20)
	CheckAllTorrent(torrentMap, lastTorrentMap)
	currentTimestamp += 2
	processCIDRTestPeer(ip, 6881, .1, 0)
	if peers, ips := CheckAllTorrent(torrentMap, lastTorrentMap); peers+ips != 0 {
		t.Fatal("compared progress across sessions")
	}
	currentTimestamp += 2
	processCIDRTestPeer(ip, 6881, .1, 10<<20)
	if peers, _ := CheckAllTorrent(torrentMap, lastTorrentMap); peers != 1 {
		t.Fatalf("current-session violation bans=%d", peers)
	}
}
