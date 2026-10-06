package stats

import "testing"

func TestCleanHistoryExpiresIndividualPortsAndKeepsTraffic(t *testing.T) {
	f := newStatisticsFixture()
	f.now = 100
	s := f.store
	f.update(func(c *Settings) {
		c.HistoryRetention = 120
		c.Interval = 1
		c.MaxIPPortCount = 1
	})
	const ip = "203.0.113.10"
	f.observe(ip, 6881, .5, 100<<20)
	f.now = 200
	f.observe(ip, 6882, .5, 20<<20)
	DeepCopyIPMap(s.State().IPMap, s.State().LastIPMap)
	DeepCopyTorrentMap(s.State().TorrentMap, s.State().LastTorrentMap)
	f.now = 250
	s.CleanHistory()
	info, peer := s.State().IPMap[ip], s.State().TorrentMap["hash"].Peers[ip]
	if len(info.Port) != 1 || !info.Port[6882] || len(info.TorrentPeers["hash"]) != 1 {
		t.Fatalf("IP ports/raw samples not pruned: %+v", info)
	}
	if len(peer.Port) != 1 || !peer.Port[6882] || len(peer.Connections) != 1 {
		t.Fatalf("torrent ports/connections not pruned: %+v", peer)
	}
	if _, exists := s.State().LastTorrentMap["hash"].Peers[ip].Connections[6881]; exists {
		t.Fatal("expired connection retained a comparison baseline")
	}
	if info.TorrentUploaded["hash"] != 120<<20 || peer.Uploaded != 120<<20 {
		t.Fatalf("expiry discarded accumulated traffic: IP=%d torrent=%d", info.TorrentUploaded["hash"], peer.Uploaded)
	}
	if count := s.CheckAllIP(); count != 0 {
		t.Fatalf("expired port caused %d bans", count)
	}
}

func TestPruneIPPortsKeepsPortsActiveInAnotherTorrent(t *testing.T) {
	info := IPInfoStruct{
		Port: map[int]bool{6881: true, 6882: true},
		TorrentPeers: map[string]map[int]PeerTrafficCounter{
			"old": {6881: {LastSeen: 100}, 6882: {LastSeen: 100}},
			"new": {6881: {LastSeen: 200}},
		},
	}
	PruneIPPorts(&info, 250, 120)
	if len(info.Port) != 1 || !info.Port[6881] || len(info.TorrentPeers) != 1 {
		t.Fatalf("active port union=%v raw samples=%v", info.Port, info.TorrentPeers)
	}
}

func TestIPCheckExpiresPortsBeforeHistoryCleanup(t *testing.T) {
	f := newStatisticsFixture()
	f.now = 100
	s := f.store
	f.update(func(c *Settings) {
		c.HistoryRetention, c.Interval = 120, 1
		c.MaxIPPortCount = 1
	})
	const ip = "203.0.113.10"
	s.AddIPInfo(nil, ip, 6881, "hash", 0, 100<<20)
	f.now = 200
	s.AddIPInfo(nil, ip, 6882, "hash", 0, 20<<20)
	if len(s.State().IPMap[ip].Port) != 2 {
		t.Fatal("test requires both ports to remain live before the expiry boundary")
	}
	f.now = 250
	if count := s.CheckAllIP(); count != 0 {
		t.Fatalf("historical port caused %d bans before CleanHistory ran", count)
	}
	if len(s.State().IPMap[ip].Port) != 1 || !s.State().IPMap[ip].Port[6882] {
		t.Fatalf("IP check did not expire old port: %v", s.State().IPMap[ip].Port)
	}
}

func TestIPReturningPortStartsFreshBaselineBeforeHistoryCleanup(t *testing.T) {
	f := newStatisticsFixture()
	f.now = 100
	s := f.store
	f.update(func(c *Settings) {
		c.HistoryRetention, c.Interval = 120, 1
	})
	const ip = "203.0.113.10"
	s.AddIPInfo(nil, ip, 6881, "hash", 0, 100<<20)
	DeepCopyIPMap(s.State().IPMap, s.State().LastIPMap)
	f.now = 300
	s.AddIPInfo(nil, ip, 6881, "hash", 0, 500<<20)
	if delta := IPUploadedDelta(s.State().IPMap[ip], s.State().LastIPMap[ip]); delta != 0 {
		t.Fatalf("returning endpoint reused expired raw baseline: delta=%d", delta)
	}
	f.now++
	s.AddIPInfo(nil, ip, 6881, "hash", 0, 505<<20)
	if delta := IPUploadedDelta(s.State().IPMap[ip], s.State().LastIPMap[ip]); delta != 5<<20 {
		t.Fatalf("fresh endpoint growth=%d, want 5 MiB", delta)
	}
}

func TestPortExpiryRetentionAndLegacyRecords(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Settings
		want int64
	}{
		{"disabled", Settings{HistoryRetention: 0, Interval: 1000}, 0},
		{"configured", Settings{HistoryRetention: 120, Interval: 1}, 120},
		{"polling", Settings{HistoryRetention: 1, Interval: 100}, 200},
		{"ip", Settings{HistoryRetention: 1, IPUpCheckInterval: 100}, 200},
		{"torrent", Settings{HistoryRetention: 1, TorrentMapCleanInterval: 100}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EffectiveHistoryRetention(&tc.cfg); got != tc.want {
				t.Fatalf("retention=%d, want %d", got, tc.want)
			}
		})
	}
	ip := IPInfoStruct{Port: map[int]bool{6881: true}}
	peer := PeerInfoStruct{Port: map[int]bool{6881: true}}
	PruneIPPorts(&ip, 1000, 120)
	PruneTorrentPorts(&peer, nil, 1000, 120)
	if !ip.Port[6881] || !peer.Port[6881] {
		t.Fatal("legacy records without per-port timestamps were discarded")
	}
	ip.TorrentPeers = map[string]map[int]PeerTrafficCounter{"hash": {6881: {LastSeen: 100}}}
	peer.Connections = map[int]PeerInfoStruct{6881: {LastSeen: 100}}
	PruneIPPorts(&ip, 1000, 0)
	PruneTorrentPorts(&peer, nil, 1000, 0)
	if !ip.Port[6881] || !peer.Port[6881] {
		t.Fatal("retention=0 must disable port expiry")
	}
}

func TestTorrentReturningPortStartsFreshBaselineBeforeCleanup(t *testing.T) {
	f := newStatisticsFixture()
	f.now = 100
	s := f.store
	f.update(func(c *Settings) {
		c.HistoryRetention, c.Interval = 120, 1
		c.IPUpCheckPerTorrentRatio = 0
		c.BanByRelativeProgressUploaded = true
		c.BanByRelativePUStartMB = 1
		c.BanByRelativePUStartPercent = 1
		c.BanByRelativePUAntiErrorRatio = 1
	})
	const ip = "203.0.113.10"
	s.AddTorrentInfo("hash", 1000<<20, nil, ip, 6881, .5, 0, 100<<20, "id", "client")
	DeepCopyTorrentMap(s.State().TorrentMap, s.State().LastTorrentMap)
	f.now = 300
	// 不调用 CleanHistory: 采样过程本身必须先清除过期的旧端点.
	s.AddTorrentInfo("hash", 1000<<20, nil, ip, 6881, .5, 0, 500<<20, "id", "client")
	peer := s.State().TorrentMap["hash"].Peers[ip]
	if peer.Uploaded != 600<<20 || peer.Connections[6881].Uploaded != 500<<20 || peer.Connections[6881].FirstSeen != 300 {
		t.Fatalf("returning endpoint reused old raw counters: %+v", peer)
	}
	if _, exists := s.State().LastTorrentMap["hash"].Peers[ip].Connections[6881]; exists {
		t.Fatal("returning endpoint reused an expired baseline")
	}
	if peers, ips := s.CheckAllTorrent(); peers+ips != 0 {
		t.Fatalf("returning endpoint caused %d bans", peers+ips)
	}
}

func TestTorrentCheckSkipsExpiredConnectionsBeforeCleanup(t *testing.T) {
	f := newStatisticsFixture()
	f.now = 100
	s := f.store
	f.update(func(c *Settings) {
		c.HistoryRetention, c.Interval = 120, 1
		c.IPUpCheckPerTorrentRatio = 2
	})
	const ip = "203.0.113.10"
	s.AddTorrentInfo("hash", 100<<20, nil, ip, 6881, .01, 0, 100<<20, "id", "client")
	f.now = 200
	s.AddTorrentInfo("hash", 100<<20, nil, ip, 6882, .5, 0, 1<<20, "id", "client")
	f.now = 250
	if peers, ips := s.CheckAllTorrent(); peers+ips != 0 {
		t.Fatalf("expired endpoint caused %d bans", peers+ips)
	}
	if len(s.State().TorrentMap["hash"].Peers[ip].Connections) != 1 {
		t.Fatal("expired connection was not pruned before torrent checks")
	}
}
