package main

import (
	"strconv"
	"testing"
)

func TestBlockedPeerTrafficUpdatesWithoutRepeatingBan(t *testing.T) {
	installCIDRTest(t, "/24", "/128")
	oldExec := execPeerCommand
	webUIPeerSyncMutex.Lock()
	oldCursor, oldEvents, oldStart := webUIPeerSyncCursor, webUIPeerSyncEvents, webUIPeerSyncEventStart
	webUIPeerSyncCursor, webUIPeerSyncEvents, webUIPeerSyncEventStart = 0, nil, 0
	webUIPeerSyncMutex.Unlock()
	t.Cleanup(func() {
		execPeerCommand = oldExec
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncCursor, webUIPeerSyncEvents, webUIPeerSyncEventStart = oldCursor, oldEvents, oldStart
		webUIPeerSyncMutex.Unlock()
	})
	commands := 0
	execPeerCommand = func(string) (bool, string, string) {
		commands++
		return true, "", ""
	}
	UpdateConfig(func(c *ConfigStruct) {
		c.PortBlockList = []uint32{6881}
		c.ExecCommand_Ban = "test-ban"
		c.WebUI = true
	})
	ip := "203.0.113.10"
	if count := processCIDRTestPeer(ip, 6881, .1, 100); count != 1 {
		t.Fatalf("initial bans=%d, want 1", count)
	}
	initial := blockPeerMap[ip]
	snapshot := GetWebUIBlockPeerSync("")
	currentTimestamp = 101
	var blocks, ipBlocks, bad, empty int
	ProcessPeer(&Peer{IP: ip, Port: 6881, ID: "new-id", Client: "new-client", DlSpeed: 1,
		Downloaded: 150, Uploaded: 300, Progress: .2}, "hash", 100<<20, &blocks, &ipBlocks, &bad, &empty)
	peer := blockPeerMap[ip]
	if peer.Downloaded != 150 || peer.Uploaded != 300 || peer.Timestamp != 101 {
		t.Fatalf("observed blocked traffic was not updated: %+v", peer)
	}
	if peer.Module != initial.Module || peer.Reason != initial.Reason || peer.InfoHash != initial.InfoHash || peer.ID != initial.ID || peer.Client != initial.Client {
		t.Fatal("traffic observation changed the original ban metadata")
	}
	if commands != 1 || blocks != 0 || ipBlocks != 0 || blockCIDRMap["203.0.113.0/24"].Timestamp != 100 {
		t.Fatal("traffic observation repeated a ban action")
	}
	updated := GetWebUIBlockPeerSync(strconv.FormatUint(snapshot.Cursor, 10))
	if len(updated.Peers) != 1 || updated.Peers[0].Uploaded != 300 || updated.Peers[0].Downloaded != 150 {
		t.Fatalf("WebUI did not receive the updated traffic: %+v", updated)
	}
}

func TestBlockedPeerTrafficAccumulatesSamplesAndCounterResets(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	ip := "203.0.113.10"
	AddBlockPeer("test", "initial", ip, 6881, "hash", "id", "client", 50, 100)
	for _, sample := range []struct {
		hash          string
		uploaded, sum int64
	}{
		{"hash", 300, 300},
		{"hash", 300, 300},
		{"hash", 20, 320},
		{"other-hash", 40, 360},
	} {
		currentTimestamp++
		status, _ := CheckPeer(ip, 6881, "id", "client", 1, 1, .2, sample.uploaded/2, sample.uploaded, sample.hash, 100<<20)
		peer := blockPeerMap[ip]
		if status != 2 || peer.Uploaded != sample.sum || peer.Downloaded != sample.sum/2 {
			t.Fatalf("sample=%+v status=%d totals=%d/%d", sample, status, peer.Downloaded, peer.Uploaded)
		}
	}
}

func TestBlockedPeerTrafficKeepsPortCountersSeparate(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	ip := "203.0.113.10"
	AddBlockPeer("test", "initial", ip, 6881, "hash", "id", "client", 50, 100)
	for _, sample := range []struct {
		port          int
		uploaded, sum int64
	}{
		{6882, 300, 400},
		{6881, 100, 400},
		{6882, 300, 400},
		{6881, 150, 450},
		{6882, 20, 470},
		{6882, 20, 470},
	} {
		currentTimestamp++
		status, _ := CheckPeer(ip, sample.port, "id", "client", 1, 1, .2, sample.uploaded/2, sample.uploaded, "hash", 100<<20)
		peer := blockPeerMap[ip]
		if status != 2 || peer.Uploaded != sample.sum || peer.Downloaded != sample.sum/2 {
			t.Fatalf("sample=%+v status=%d totals=%d/%d", sample, status, peer.Downloaded, peer.Uploaded)
		}
	}
}

func TestBlockedPeerTrafficSeedsStatisticsWithoutRecounting(t *testing.T) {
	for _, hash := range []string{"", "hash"} {
		t.Run("aggregate-"+hash, func(t *testing.T) {
			installCIDRTest(t, "/32", "/128")
			ip := "203.0.113.10"
			AddBlockPeer("statistics", "initial", ip, -1, hash, "id", "client", 150, 300)
			counters := map[string]map[int]PeerTrafficCounter{
				"hash": {
					6881: {Downloaded: 50, Uploaded: 100, LastSeen: 100},
					6882: {Downloaded: 100, Uploaded: 200, LastSeen: 100},
				},
			}
			SeedBlockedPeerCounters(ip, counters)
			// 后续统计快照变化不得修改已经安装的封禁基线。
			counters["hash"][6881] = PeerTrafficCounter{Uploaded: 999}
			for _, sample := range []struct {
				port          int
				uploaded, sum int64
			}{
				{6881, 100, 300},
				{6882, 200, 300},
				{6881, 120, 320},
				{6882, 20, 340},
				{6883, 60, 400},
			} {
				currentTimestamp++
				CheckPeer(ip, sample.port, "id", "client", 1, 1, .2, sample.uploaded/2, sample.uploaded, "hash", 100<<20)
				peer := blockPeerMap[ip]
				if peer.Uploaded != sample.sum || peer.Downloaded != sample.sum/2 {
					t.Fatalf("sample=%+v totals=%d/%d", sample, peer.Downloaded, peer.Uploaded)
				}
			}
		})
	}
}

func TestBlockedPeerTrafficSeedingPreservesOtherPortBaselines(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	ip := "203.0.113.10"
	for _, sample := range []struct {
		port int
		raw  int64
	}{{6881, 20}, {6882, 30}} {
		// 统计累计量包含重连前的数据，当前连接的原始计数较小。
		AddBlockPeer("statistics", "initial", ip, sample.port, "hash", "id", "client", sample.raw*5, sample.raw*10)
		SeedBlockedPeerCounters(ip, map[string]map[int]PeerTrafficCounter{
			"hash": {sample.port: {Downloaded: sample.raw / 2, Uploaded: sample.raw, LastSeen: currentTimestamp}},
		})
	}
	for _, sample := range []struct {
		port int
		raw  int64
	}{{6881, 20}, {6882, 30}} {
		currentTimestamp++
		CheckPeer(ip, sample.port, "id", "client", 1, 1, .2, sample.raw/2, sample.raw, "hash", 100<<20)
	}
	if peer := blockPeerMap[ip]; peer.Downloaded != 250 || peer.Uploaded != 500 {
		t.Fatalf("seeding the second port discarded the first baseline: totals=%d/%d", peer.Downloaded, peer.Uploaded)
	}
}

func TestBlockedPeerTrafficUnknownSamplePreservesKnownBaseline(t *testing.T) {
	for _, initial := range []int64{100, -1} {
		t.Run(strconv.FormatInt(initial, 10), func(t *testing.T) {
			installCIDRTest(t, "/32", "/128")
			ip := "203.0.113.10"
			AddBlockPeer("test", "initial", ip, 6881, "hash", "id", "client", initial, initial)
			for _, sample := range []struct {
				downloaded, uploaded int64
			}{{-1, -1}, {110, -1}, {-1, 110}, {110, 110}} {
				currentTimestamp++
				CheckPeer(ip, 6881, "id", "client", 1, 1, .2, sample.downloaded, sample.uploaded, "hash", 100<<20)
			}
			if peer := blockPeerMap[ip]; peer.Downloaded != 110 || peer.Uploaded != 110 {
				t.Fatalf("unknown sample discarded the last valid baseline: totals=%d/%d", peer.Downloaded, peer.Uploaded)
			}
		})
	}
}

func TestStatisticsBanPreservesObservedConnectionBaselines(t *testing.T) {
	for _, rule := range []string{"global", "torrent"} {
		t.Run(rule, func(t *testing.T) {
			installCIDRTest(t, "/32", "/128")
			UpdateConfig(func(c *ConfigStruct) {
				c.MaxIPPortCount = 0
				c.IPUpCheckIncrementMB = 1
				c.IPUpCheckPerTorrentRatio = 3
			})
			ip := "203.0.113.10"
			processCIDRTestPeer(ip, 6881, .1, 100<<20)
			uploaded := int64(100 << 20)
			if rule == "global" {
				CheckAllIP(ipMap, lastIPMap)
				currentTimestamp += 2
				uploaded = 103 << 20
				processCIDRTestPeer(ip, 6881, .1, uploaded)
				CheckAllIP(ipMap, lastIPMap)
			} else {
				CheckAllTorrent(torrentMap, lastTorrentMap)
			}
			if got := blockPeerMap[ip].Uploaded; got != uploaded {
				t.Fatalf("initial ban total=%d want=%d", got, uploaded)
			}
			processCIDRTestPeer(ip, 6881, .1, uploaded)
			if got := blockPeerMap[ip].Uploaded; got != uploaded {
				t.Fatalf("repeated sample counted twice: %d", got)
			}
			processCIDRTestPeer(ip, 6881, .1, uploaded+(1<<20))
			if got := blockPeerMap[ip].Uploaded; got != uploaded+(1<<20) {
				t.Fatalf("subsequent growth=%d", got)
			}
		})
	}
}
