package stats

import (
	"fmt"
	"testing"
)

func TestHistoryChurnStaysBounded(t *testing.T) {
	f := newStatisticsFixture()
	f.now = 100
	f.settings.HistoryRetention, f.settings.HistoryMaxEntries = 120, 200
	f.settings.Interval, f.settings.MaxIPPortCount, f.settings.IPUpCheckIncrementMB = 1, 10, 1
	f.settings.BanByRelativeProgressUploaded = true
	s := f.store
	for cycle := 0; cycle < 50; cycle++ {
		for i := 0; i < 100; i++ {
			ip, hash := fmt.Sprintf("peer-%d-%d", cycle, i), fmt.Sprintf("hash-%d", cycle)
			s.AddIPInfo(nil, ip, 6881, hash, 0, 10<<20)
			s.AddIPInfo(nil, "long-lived-ip", 6881, fmt.Sprintf("%s-%d", hash, i), 0, 10<<20)
			s.AddTorrentInfo(hash, 100<<20, nil, ip, 6881, .1, 0, 10<<20, "", "")
		}
		DeepCopyIPMap(s.State().IPMap, s.State().LastIPMap)
		DeepCopyTorrentMap(s.State().TorrentMap, s.State().LastTorrentMap)
		s.CleanHistory()
		peerCount, nestedCount := 0, 0
		for _, info := range s.State().TorrentMap {
			peerCount += len(info.Peers)
		}
		for _, info := range s.State().IPMap {
			nestedCount += len(info.TorrentUploaded)
		}
		if len(s.State().IPMap) > 200 || len(s.State().LastIPMap) > 200 || peerCount > 200 || nestedCount > 200 {
			t.Fatalf("unbounded cycle %d: ips=%d baseline=%d peers=%d counters=%d", cycle, len(s.State().IPMap), len(s.State().LastIPMap), peerCount, nestedCount)
		}
		f.now += 61
	}
	f.now += 121
	s.CleanHistory()
	if len(s.State().IPMap)+len(s.State().LastIPMap)+len(s.State().TorrentMap)+len(s.State().LastTorrentMap) != 0 {
		t.Fatal("inactive history and baseline were not reclaimed")
	}
}

func TestPrunedTorrentStartsWithFreshBaseline(t *testing.T) {
	f := newStatisticsFixture()
	f.now = 100
	f.settings.HistoryRetention, f.settings.HistoryMaxEntries = 120, 200
	f.settings.Interval, f.settings.MaxIPPortCount, f.settings.IPUpCheckIncrementMB = 1, 10, 1
	f.settings.BanByRelativeProgressUploaded = true
	s := f.store
	s.AddIPInfo(nil, "peer", 6881, "old", 0, 10<<20)
	DeepCopyIPMap(s.State().IPMap, s.State().LastIPMap)
	f.now = 221
	s.AddIPInfo(nil, "peer", 6881, "active", 0, 1<<20)
	s.CleanHistory()
	if _, exists := s.State().IPMap["peer"].TorrentUploaded["old"]; exists {
		t.Fatal("inactive nested counter retained")
	}
	s.AddIPInfo(nil, "peer", 6881, "old", 0, 100<<20)
	if delta := s.IsIPTooHighUploaded(s.State().IPMap["peer"], s.State().LastIPMap["peer"]); delta != 0 {
		t.Fatalf("reappearance counted as %d MB of new traffic", delta)
	}
	DeepCopyIPMap(s.State().IPMap, s.State().LastIPMap)
	s.AddIPInfo(nil, "peer", 6881, "old", 0, 103<<20)
	if delta := s.IsIPTooHighUploaded(s.State().IPMap["peer"], s.State().LastIPMap["peer"]); delta != 3 {
		t.Fatalf("new baseline delta=%d, want 3", delta)
	}
}

func TestHistoryRetentionProtectsDetectionWindow(t *testing.T) {
	f := newStatisticsFixture()
	f.now = 100
	f.settings.HistoryRetention, f.settings.HistoryMaxEntries = 120, 200
	f.settings.Interval, f.settings.MaxIPPortCount, f.settings.IPUpCheckIncrementMB = 1, 10, 1
	f.settings.BanByRelativeProgressUploaded = true
	s := f.store
	f.update(func(c *Settings) { c.HistoryRetention = 1; c.IPUpCheckInterval = 100 })
	s.AddIPInfo(nil, "peer", 6881, "hash", 0, 1)
	f.now = 250
	s.CleanHistory()
	if len(s.State().IPMap) != 1 {
		t.Fatal("history expired inside the detection window")
	}
}
