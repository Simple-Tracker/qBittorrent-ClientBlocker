package stats

import (
	"net"
	"reflect"
	"sync"
	"testing"
)

type statisticsFixture struct {
	store    *Store
	settings Settings
	now      int64
	blocked  map[string]map[int]bool
	events   []BlockEvent
}

func newStatisticsFixture() *statisticsFixture {
	f := &statisticsFixture{now: 1000, settings: Settings{IPUpCheckInterval: 1, TorrentMapCleanInterval: 1, MaxIPPortCount: 1, IPUploadedCheck: true, IPUpCheckIncrementMB: 1000, IPUpCheckPerTorrentRatio: 1000}, blocked: make(map[string]map[int]bool)}
	f.store = NewStore(Options{
		Settings:  func() Settings { return f.settings },
		Now:       func() int64 { return f.now },
		IsBlocked: func(ip string, port int) bool { return f.blocked[ip][-1] || f.blocked[ip][port] },
		Block: func(event BlockEvent) {
			f.events = append(f.events, event)
			if f.blocked[event.IP] == nil {
				f.blocked[event.IP] = make(map[int]bool)
			}
			f.blocked[event.IP][event.Port] = true
		},
	})
	return f
}
func (f *statisticsFixture) update(fn func(*Settings)) { fn(&f.settings) }
func (f *statisticsFixture) observe(ip string, port int, progress float64, uploaded int64) {
	f.store.AddIPInfo(nil, ip, port, "hash", uploaded/2, uploaded, ip)
	f.store.AddTorrentInfo("hash", 100<<20, nil, ip, port, progress, uploaded/2, uploaded, ip, "test")
}

func TestStoreSnapshotsDoNotExposeMutableHistory(t *testing.T) {
	f := newStatisticsFixture()
	_, network, _ := net.ParseCIDR("203.0.113.0/24")
	f.store.AddIPInfo(network, "203.0.113.1", 1, "hash", 10, 20)
	f.store.AddTorrentInfo("hash", 100, network, "203.0.113.1", 1, .5, 10, 20, "id", "client")
	ip := f.store.IPSnapshot()["203.0.113.1"]
	ip.Port[2] = true
	ip.TorrentPeers["hash"][1] = PeerTrafficCounter{Uploaded: 999}
	ip.TorrentUploaded["hash"] = 999
	ip.Net.IP[0] = 1
	peer := f.store.TorrentSnapshot()["hash"].Peers["203.0.113.1"]
	peer.Connections[1] = PeerInfoStruct{Uploaded: 999}
	peer.Port[2] = true
	peer.Net.IP[0] = 2
	if got := f.store.IPSnapshot()["203.0.113.1"]; got.TorrentUploaded["hash"] != 20 || got.TorrentPeers["hash"][1].Uploaded != 20 || got.Port[2] || got.Net.String() != "203.0.113.0/24" {
		t.Fatalf("IP snapshot changed history: %+v", got)
	}
	if got := f.store.TorrentSnapshot()["hash"].Peers["203.0.113.1"]; got.Connections[1].Uploaded != 20 || got.Port[2] || got.Net.String() != "203.0.113.0/24" {
		t.Fatalf("torrent snapshot changed history: %+v", got)
	}
}

func TestStoreRestorePreservesPolicyAndBaselines(t *testing.T) {
	f := newStatisticsFixture()
	f.observe("203.0.113.1", 1, .5, 10)
	f.store.CheckAllIP()
	state := f.store.Snapshot()
	f.observe("203.0.113.1", 1, .5, 50)
	f.store.ReplaceState(state)
	f.now += 2
	f.observe("203.0.113.1", 1, .5, 20)
	if got := IPUploadedDelta(f.store.State().IPMap["203.0.113.1"], f.store.State().LastIPMap["203.0.113.1"]); got != 10 {
		t.Fatalf("restored delta = %d", got)
	}
	if !f.store.HasPeer("203.0.113.1", "hash", 1) || f.store.HasPeer("203.0.113.1", "hash", 2) {
		t.Fatal("connection lookup mismatch")
	}
	if got := f.store.TasksForIP("203.0.113.1"); !reflect.DeepEqual(got, []string{"hash"}) {
		t.Fatalf("tasks = %v", got)
	}
	f.store.ReplaceState(nil)
	if f.store.HasPeer("203.0.113.1", "hash", 1) || len(f.store.TasksForIP("203.0.113.1")) != 0 {
		t.Fatal("replace left old history")
	}
	f.observe("203.0.113.1", 1, .5, 20)
	if len(f.store.TorrentSnapshot()) != 1 {
		t.Fatal("replace discarded sampling settings")
	}
}

func TestStoreEmitsCIDRViolationWithConnectionBaselines(t *testing.T) {
	f := newStatisticsFixture()
	f.settings.MaxIPPortCount = 0
	f.settings.IPUpCheckIncrementMB = 2
	f.store.options.CIDR = func(ip string) *net.IPNet { _, cidr, _ := net.ParseCIDR(ip + "/24"); return cidr }
	for _, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		f.observe(ip, 1, .5, 1<<20)
	}
	f.store.CheckAllIP()
	f.now += 2
	for _, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		f.observe(ip, 1, .5, 3<<20)
	}
	if got := f.store.CheckAllIP(); got != 2 {
		t.Fatalf("CIDR bans = %d", got)
	}
	for _, event := range f.events {
		if event.Net.String() != "203.0.113.0/24" || event.Counters["hash"][1].Uploaded != 3<<20 || event.Uploaded != 3<<20 {
			t.Fatalf("incomplete event: %+v", event)
		}
		event.Counters["hash"][1] = PeerTrafficCounter{}
		if f.store.IPSnapshot()[event.IP].TorrentPeers["hash"][1].Uploaded != 3<<20 {
			t.Fatal("event counter aliases sampling history")
		}
	}
}

func TestStoreConcurrentSamplingSnapshotsAndChecks(t *testing.T) {
	s := NewStore(Options{Settings: func() Settings {
		return Settings{IPUploadedCheck: true, IPUpCheckIncrementMB: 1000, IPUpCheckPerTorrentRatio: 1000}
	}, Now: func() int64 { return 1000 }})
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := int64(0); i < 50; i++ {
				s.AddIPInfo(nil, "203.0.113.1", 1, "hash", i, i)
				s.AddTorrentInfo("hash", 1000000, nil, "203.0.113.1", 1, .5, i, i, "id", "test")
				s.CheckAllIP()
				s.CheckAllTorrent()
				s.CleanHistory()
				s.IPSnapshot()
				s.TorrentSnapshot()
				s.Snapshot()
			}
		}()
	}
	wg.Wait()
}

func TestStoreHasPeerSupportsEitherSamplingPath(t *testing.T) {
	f := newStatisticsFixture()
	f.store.AddIPInfo(nil, "203.0.113.1", 1, "ip-only", 10, 20)
	f.store.AddTorrentInfo("torrent-only", 100, nil, "203.0.113.2", 2, .5, 10, 20, "id", "client")
	if !f.store.HasPeer("203.0.113.1", "ip-only", 1) || !f.store.HasPeer("203.0.113.2", "torrent-only", 2) {
		t.Fatal("idle sample would be lost when only one statistics path is enabled")
	}
	f.store.State().TorrentMap["legacy"] = TorrentInfoStruct{Peers: map[string]PeerInfoStruct{"203.0.113.3": {Port: map[int]bool{3: true}}}}
	if f.store.HasPeer("203.0.113.3", "legacy", 3) {
		t.Fatal("legacy port presence is not a retained connection sample")
	}
	if got := f.store.TasksForIP("203.0.113.2"); len(got) != 0 {
		t.Fatalf("torrent-only sample extended IP task history: %v", got)
	}
}
