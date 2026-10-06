package app

import (
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/stats"
)

func TestBTNHistoryReportsConnectionObservationTimes(t *testing.T) {
	reports := installBTNProtocolTest(t)
	ip := "203.0.113.10"
	observe := func(at int64, port int, uploaded int64) {
		currentTimestamp = at
		statistics.AddTorrentInfo("hash", 1000, nil, ip, port, .5, uploaded/2, uploaded, "peer", "client")
	}
	observe(100, 6881, 100)
	observe(110, 6882, 200)
	observe(120, 6881, 150)
	// 计数重置后仍属于同一条保留的连接历史.
	observe(130, 6881, 20)
	for _, reportedAt := range []int64{200, 300} {
		BTN_SubmitHistories(statistics.State().TorrentMap, statistics.State().LastTorrentMap, reportedAt)
		report := <-reports
		if len(report.histories) != 2 {
			t.Fatalf("reported %d connections, want 2", len(report.histories))
		}
		for _, peer := range report.histories {
			first, last := int64(100000), int64(130000)
			if peer.PeerPort == 6882 {
				first, last = 110000, 110000
			} else if peer.PeerPort != 6881 {
				t.Fatalf("unexpected port %d", peer.PeerPort)
			}
			if peer.FirstTimeSeen != first || peer.LastTimeSeen != last {
				t.Fatalf("port %d reported at %d: observed times=%d/%d, want %d/%d", peer.PeerPort, reportedAt, peer.FirstTimeSeen, peer.LastTimeSeen, first, last)
			}
		}
	}
	if info := statistics.State().TorrentMap["hash"].Peers[ip]; info.FirstSeen != 100 || info.LastSeen != 130 {
		t.Fatalf("IP observation bounds=%d/%d, want 100/130", info.FirstSeen, info.LastSeen)
	}
}

func TestBTNHistoryMissingObservationTimesUseKnownTimestamp(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		first, last, wantFirst, wantLast int64
	}{
		{"only last", 0, 100, 100000, 100000},
		{"only first", 100, 0, 100000, 100000},
		{"legacy unknown", 0, 0, 200000, 200000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reports := installBTNProtocolTest(t)
			statistics.State().TorrentMap["hash"] = stats.TorrentInfoStruct{Peers: map[string]stats.PeerInfoStruct{
				"203.0.113.10": {Port: map[int]bool{6881: true}, FirstSeen: tc.first, LastSeen: tc.last},
			}}
			BTN_SubmitHistories(statistics.State().TorrentMap, statistics.State().LastTorrentMap, 200)
			report := <-reports
			if len(report.histories) != 1 {
				t.Fatalf("reported %d peers, want 1", len(report.histories))
			}
			peer := report.histories[0]
			if peer.FirstTimeSeen != tc.wantFirst || peer.LastTimeSeen != tc.wantLast {
				t.Fatalf("observed times=%d/%d, want %d/%d", peer.FirstTimeSeen, peer.LastTimeSeen, tc.wantFirst, tc.wantLast)
			}
		})
	}
}
