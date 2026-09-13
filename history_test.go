package main

import (
	"fmt"
	"testing"
)

func InstallHistoryTest(t *testing.T) {
	t.Helper()
	oldCfg := ConfigSnapshot()
	oldIP, oldLastIP, oldTorrent, oldLastTorrent := ipMap, lastIPMap, torrentMap, lastTorrentMap
	oldNow, oldClean := currentTimestamp, lastHistoryCleanTimestamp
	t.Cleanup(func() {
		ReplaceConfig(oldCfg)
		ipMap, lastIPMap, torrentMap, lastTorrentMap = oldIP, oldLastIP, oldTorrent, oldLastTorrent
		currentTimestamp, lastHistoryCleanTimestamp = oldNow, oldClean
	})
	cfg := *oldCfg
	cfg.HistoryRetention, cfg.HistoryMaxEntries = 120, 200
	cfg.Interval, cfg.IPUpCheckInterval, cfg.TorrentMapCleanInterval = 1, 1, 1
	cfg.MaxIPPortCount, cfg.IPUpCheckIncrementMB = 10, 1
	cfg.IPUploadedCheck, cfg.BanByRelativeProgressUploaded = true, true
	ReplaceConfig(&cfg)
	ipMap, lastIPMap = make(map[string]IPInfoStruct), make(map[string]IPInfoStruct)
	torrentMap, lastTorrentMap = make(map[string]TorrentInfoStruct), make(map[string]TorrentInfoStruct)
	currentTimestamp, lastHistoryCleanTimestamp = 100, 0
}

func TestHistoryChurnStaysBounded(t *testing.T) {
	InstallHistoryTest(t)
	for cycle := 0; cycle < 50; cycle++ {
		for i := 0; i < 100; i++ {
			ip, hash := fmt.Sprintf("peer-%d-%d", cycle, i), fmt.Sprintf("hash-%d", cycle)
			AddIPInfo(nil, ip, 6881, hash, 0, 10<<20)
			AddIPInfo(nil, "long-lived-ip", 6881, fmt.Sprintf("%s-%d", hash, i), 0, 10<<20)
			AddTorrentInfo(hash, 100<<20, nil, ip, 6881, .1, 0, 10<<20, "", "")
		}
		DeepCopyIPMap(ipMap, lastIPMap)
		DeepCopyTorrentMap(torrentMap, lastTorrentMap)
		CleanHistory()
		peerCount, nestedCount := 0, 0
		for _, info := range torrentMap {
			peerCount += len(info.Peers)
		}
		for _, info := range ipMap {
			nestedCount += len(info.TorrentUploaded)
		}
		if len(ipMap) > 200 || len(lastIPMap) > 200 || peerCount > 200 || nestedCount > 200 {
			t.Fatalf("unbounded cycle %d: ips=%d baseline=%d peers=%d counters=%d", cycle, len(ipMap), len(lastIPMap), peerCount, nestedCount)
		}
		currentTimestamp += 61
	}
	currentTimestamp += 121
	CleanHistory()
	if len(ipMap)+len(lastIPMap)+len(torrentMap)+len(lastTorrentMap) != 0 {
		t.Fatal("inactive history and baseline were not reclaimed")
	}
}

func TestPrunedTorrentStartsWithFreshBaseline(t *testing.T) {
	InstallHistoryTest(t)
	AddIPInfo(nil, "peer", 6881, "old", 0, 10<<20)
	DeepCopyIPMap(ipMap, lastIPMap)
	currentTimestamp = 221
	AddIPInfo(nil, "peer", 6881, "active", 0, 1<<20)
	CleanHistory()
	if _, exists := ipMap["peer"].TorrentUploaded["old"]; exists {
		t.Fatal("inactive nested counter retained")
	}
	AddIPInfo(nil, "peer", 6881, "old", 0, 100<<20)
	if delta := IsIPTooHighUploaded(ipMap["peer"], lastIPMap["peer"]); delta != 0 {
		t.Fatalf("reappearance counted as %d MB of new traffic", delta)
	}
	DeepCopyIPMap(ipMap, lastIPMap)
	AddIPInfo(nil, "peer", 6881, "old", 0, 103<<20)
	if delta := IsIPTooHighUploaded(ipMap["peer"], lastIPMap["peer"]); delta != 3 {
		t.Fatalf("new baseline delta=%d, want 3", delta)
	}
}

func TestHistoryRetentionProtectsDetectionWindow(t *testing.T) {
	InstallHistoryTest(t)
	UpdateConfig(func(c *ConfigStruct) { c.HistoryRetention = 1; c.IPUpCheckInterval = 100 })
	AddIPInfo(nil, "peer", 6881, "hash", 0, 1)
	currentTimestamp = 250
	CleanHistory()
	if len(ipMap) != 1 {
		t.Fatal("history expired inside the detection window")
	}
}
