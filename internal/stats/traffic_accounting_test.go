package stats

import "testing"

func TestPeerTrafficCountersSeparatePortsAndSamplingOrder(t *testing.T) {
	f := newStatisticsFixture()
	s := f.store
	f.update(func(c *Settings) { c.MaxIPPortCount = 0; c.IPUpCheckIncrementMB = 10 })
	ip := "203.0.113.10"
	f.observe(ip, 6881, .5, 100<<20)
	f.observe(ip, 6882, .5, 20<<20)
	s.CheckAllIP()
	DeepCopyTorrentMap(s.State().TorrentMap, s.State().LastTorrentMap)
	f.now += 2
	f.observe(ip, 6882, .5, 21<<20)
	f.observe(ip, 6881, .5, 101<<20)
	if delta := IPUploadedDelta(s.State().IPMap[ip], s.State().LastIPMap[ip]); delta != 2<<20 {
		t.Fatalf("delta=%d, want 2 MiB", delta)
	}
	if n := s.CheckAllIP(); n != 0 {
		t.Fatalf("false bans=%d", n)
	}
	if got := s.State().TorrentMap["hash"].Peers[ip].Uploaded; got != 122<<20 {
		t.Fatalf("total=%d, want 122 MiB", got)
	}
	if got := s.State().LastTorrentMap["hash"].Peers[ip].Connections[6881].Uploaded; got != 100<<20 {
		t.Fatal("connection baseline aliases current sample")
	}
}

func TestPeerTrafficRetainsObservedGrowthBeforeCounterReset(t *testing.T) {
	f := newStatisticsFixture()
	s := f.store
	f.update(func(c *Settings) { c.MaxIPPortCount = 0 })
	ip := "203.0.113.10"
	f.observe(ip, 6881, .5, 100<<20)
	s.CheckAllIP()
	f.now++
	f.observe(ip, 6881, .5, 150<<20)
	f.observe(ip, 6881, .1, 10<<20)
	if delta := IPUploadedDelta(s.State().IPMap[ip], s.State().LastIPMap[ip]); delta != 60<<20 {
		t.Fatalf("delta=%d, want 60 MiB", delta)
	}
	peer := s.State().TorrentMap["hash"].Peers[ip].Connections[6881]
	if peer.Uploaded != 160<<20 || peer.RawUploaded != 10<<20 {
		t.Fatalf("cumulative=%d raw=%d", peer.Uploaded, peer.RawUploaded)
	}
}

func TestNewPortEstablishesIndependentUploadBaseline(t *testing.T) {
	f := newStatisticsFixture()
	s := f.store
	f.update(func(c *Settings) { c.MaxIPPortCount = 0; c.IPUpCheckIncrementMB = 1 })
	ip := "203.0.113.10"
	f.observe(ip, 6881, .5, 10<<20)
	s.CheckAllIP()
	f.now += 2
	f.observe(ip, 6882, .5, 1000<<20)
	if n := s.CheckAllIP(); n != 0 {
		t.Fatalf("first connection sample caused %d bans", n)
	}
	f.now += 2
	f.observe(ip, 6882, .5, 1003<<20)
	if delta := IPUploadedDelta(s.State().IPMap[ip], s.State().LastIPMap[ip]); delta != 3<<20 {
		t.Fatalf("subsequent delta=%d", delta)
	}
}

func TestTorrentRulesUseMatchingConnectionProgress(t *testing.T) {
	f := newStatisticsFixture()
	s := f.store
	f.update(func(c *Settings) { c.MaxIPPortCount = 0; c.IPUpCheckPerTorrentRatio = 2 })
	ip := "203.0.113.10"
	f.observe(ip, 6881, .8, 70<<20)
	f.observe(ip, 6882, .01, 1<<20)
	if peers, ips := s.CheckAllTorrent(); peers+ips != 0 {
		t.Fatal("mixed connection totals and progress triggered a ban")
	}
	f.now += 2
	f.observe(ip, 6881, .8, 71<<20)
	f.observe(ip, 6882, .01, 4<<20)
	if _, ips := s.CheckAllTorrent(); ips != 1 {
		t.Fatalf("per-connection violation bans=%d", ips)
	}
}

func TestUnknownTrafficCountersDoNotBecomeNegativeDeltas(t *testing.T) {
	f := newStatisticsFixture()
	s := f.store
	ip := "203.0.113.10"
	for i := 0; i < 2; i++ {
		s.AddIPInfo(nil, ip, 6881, "hash", -1, -1)
		s.AddTorrentInfo("hash", 1000, nil, ip, 6881, .5, -1, -1, "id", "client")
	}
	if got := s.State().TorrentMap["hash"].Peers[ip].Uploaded; got != -1 {
		t.Fatalf("unknown total=%d", got)
	}
	s.CheckAllIP()
	s.AddIPInfo(nil, ip, 6881, "hash", 100, 200)
	s.AddTorrentInfo("hash", 1000, nil, ip, 6881, .5, 100, 200, "id", "client")
	if got := s.State().TorrentMap["hash"].Peers[ip].Uploaded; got != 200 {
		t.Fatalf("known total=%d", got)
	}
	if delta := IPUploadedDelta(s.State().IPMap[ip], s.State().LastIPMap[ip]); delta != 0 {
		t.Fatalf("unknown baseline produced delta=%d", delta)
	}
}

func TestUnknownSamplePreservesLastKnownCounter(t *testing.T) {
	f := newStatisticsFixture()
	s := f.store
	ip := "203.0.113.10"
	for _, uploaded := range []int64{100, -1, 110} {
		s.AddIPInfo(nil, ip, 6881, "hash", uploaded, uploaded)
		s.AddTorrentInfo("hash", 1000, nil, ip, 6881, .5, uploaded, uploaded, "id", "client")
		if uploaded == 100 {
			DeepCopyIPMap(s.State().IPMap, s.State().LastIPMap)
		}
	}
	if got := s.State().IPMap[ip].TorrentUploaded["hash"]; got != 110 {
		t.Fatalf("IP total=%d", got)
	}
	if got := s.State().TorrentMap["hash"].Peers[ip].Uploaded; got != 110 {
		t.Fatalf("torrent total=%d", got)
	}
	if delta := IPUploadedDelta(s.State().IPMap[ip], s.State().LastIPMap[ip]); delta != 10 {
		t.Fatalf("delta=%d", delta)
	}
}

func TestNewTorrentCountsOnlyGrowthObservedInsideWindow(t *testing.T) {
	f := newStatisticsFixture()
	s := f.store
	ip := "203.0.113.10"
	s.AddIPInfo(nil, ip, 6881, "old", 0, 10)
	DeepCopyIPMap(s.State().IPMap, s.State().LastIPMap)
	s.AddIPInfo(nil, ip, 6881, "new", 0, 100)
	s.AddIPInfo(nil, ip, 6881, "new", 0, 160)
	if delta := IPUploadedDelta(s.State().IPMap[ip], s.State().LastIPMap[ip]); delta != 60 {
		t.Fatalf("delta=%d", delta)
	}
}

func TestRelativeProgressUsesCurrentSessionCounters(t *testing.T) {
	f := newStatisticsFixture()
	s := f.store
	f.update(func(c *Settings) {
		c.BanByRelativeProgressUploaded = true
		c.BanByRelativePUStartMB = 1
		c.BanByRelativePUStartPercent = 3
		c.BanByRelativePUAntiErrorRatio = 3
	})
	ip := "203.0.113.10"
	f.observe(ip, 6881, .8, 1000<<20)
	s.CheckAllTorrent()
	f.now += 2
	f.observe(ip, 6881, .1, 0)
	if peers, ips := s.CheckAllTorrent(); peers+ips != 0 {
		t.Fatal("compared progress across sessions")
	}
	f.now += 2
	f.observe(ip, 6881, .1, 10<<20)
	if peers, _ := s.CheckAllTorrent(); peers != 1 {
		t.Fatalf("current-session violation bans=%d", peers)
	}
}
