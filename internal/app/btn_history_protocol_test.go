package app

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/stats"
)

type btnProtocolReport struct {
	peers     []BTN_PeerInternalStruct
	histories []BTN_PeerHistoryStruct
}

func installBTNProtocolTest(t *testing.T) <-chan btnProtocolReport {
	t.Helper()
	installCIDRTest(t, "/32", "/128")
	oldGetting := btn_isGettingConfig.Load()
	t.Cleanup(func() { btn_isGettingConfig.Store(oldGetting) })
	btn_isGettingConfig.Store(false)
	reports := make(chan btnProtocolReport, 1)
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer reader.Close()
		var report btnProtocolReport
		if r.URL.Path == "/histories" {
			var data BTN_SubmitHistoriesStruct
			if err := json.NewDecoder(reader).Decode(&data); err != nil {
				t.Error(err)
			}
			report.histories = data.Peers
		} else {
			var data BTN_SubmitPeersStruct
			if err := json.NewDecoder(reader).Decode(&data); err != nil {
				t.Error(err)
			}
			report.peers = data.Peers
		}
		reports <- report
		w.Write([]byte("Ok."))
	}))
	endpoint := ConfigSnapshot().ClientURL
	btnStateMutex.Lock()
	btnConfig = &BTN_ConfigStruct{Ability: map[string]BTN_Ability{
		"submit_histories": {Endpoint: endpoint + "/histories"},
		"submit_peers":     {Endpoint: endpoint + "/peers"},
	}}
	btnStateMutex.Unlock()
	return reports
}

func TestBTNReportsKeepConnectionIdentityAndProtocolCounters(t *testing.T) {
	reports := installBTNProtocolTest(t)
	connections := map[int]stats.PeerInfoStruct{
		6881: {ID: "peer-a", Client: "client-a", Progress: .3, Downloaded: 150, Uploaded: 300, RawDownloaded: 50, RawUploaded: 100},
		6882: {ID: "peer-b", Client: "client-b", Progress: .7, Downloaded: 70, Uploaded: 140, RawDownloaded: 70, RawUploaded: 140},
		6883: {ID: "peer-c", Client: "client-c", Downloaded: -1, Uploaded: -1, RawDownloaded: -1, RawUploaded: -1},
	}
	statistics.State().TorrentMap["hash"] = stats.TorrentInfoStruct{Size: 1000, Peers: map[string]stats.PeerInfoStruct{
		"203.0.113.10": {Port: map[int]bool{6881: true, 6882: true, 6883: true}, Connections: connections, Downloaded: 220, Uploaded: 440},
	}}
	// 协议 3 区分累计历史记录和下载器原始计数:
	// https://github.com/PBH-BTN/BTN-Spec/blob/9c4781a261c0cc6b4f1006f6807f7538e18a22ee/README.md
	for cycle := 0; cycle < 3; cycle++ {
		if cycle == 2 {
			peer := connections[6881]
			peer.RawDownloaded, peer.RawUploaded = 10, 20
			peer.Downloaded, peer.Uploaded = 160, 320
			connections[6881] = peer
		}
		// 刷新检测快照或重复上报时, 不得将原始计数清零.
		stats.DeepCopyTorrentMap(statistics.State().TorrentMap, statistics.State().LastTorrentMap)
		BTN_SubmitPeers(statistics.State().TorrentMap, currentTimestamp)
		peerReport := <-reports
		if len(peerReport.peers) != len(connections) {
			t.Fatalf("snapshot reported %d peers, want %d connections", len(peerReport.peers), len(connections))
		}
		for _, got := range peerReport.peers {
			want, exists := connections[got.PeerPort]
			if !exists || got.IPAddress != "203.0.113.10" || got.PeerID != want.ID || got.ClientName != want.Client || got.PeerProgress != want.Progress || got.Downloaded != want.RawDownloaded || got.Uploaded != want.RawUploaded {
				t.Fatalf("snapshot mixed connection fields: %+v", got)
			}
		}
		BTN_SubmitHistories(statistics.State().TorrentMap, statistics.State().LastTorrentMap, currentTimestamp)
		historyReport := <-reports
		if len(historyReport.histories) != len(connections) {
			t.Fatalf("history reported %d peers, want %d connections", len(historyReport.histories), len(connections))
		}
		for _, got := range historyReport.histories {
			want, exists := connections[got.PeerPort]
			if !exists || got.IPAddress != "203.0.113.10" || got.PeerID != want.ID || got.ClientName != want.Client || got.PeerProgress != want.Progress || got.Downloaded != want.Downloaded || got.Uploaded != want.Uploaded || got.DownloadedOffset != want.RawDownloaded || got.UploadedOffset != want.RawUploaded {
				t.Fatalf("history mixed cumulative and raw counters: %+v", got)
			}
		}
	}
}

func TestBTNHistoryLegacyRecordDoesNotUseDetectionBaseline(t *testing.T) {
	reports := installBTNProtocolTest(t)
	statistics.State().TorrentMap["hash"] = stats.TorrentInfoStruct{Peers: map[string]stats.PeerInfoStruct{
		"203.0.113.10": {Port: map[int]bool{6881: true}, Downloaded: 150, Uploaded: 300},
	}}
	stats.DeepCopyTorrentMap(statistics.State().TorrentMap, statistics.State().LastTorrentMap)
	BTN_SubmitHistories(statistics.State().TorrentMap, statistics.State().LastTorrentMap, currentTimestamp)
	report := <-reports
	if len(report.histories) != 1 {
		t.Fatalf("history reported %d peers, want 1", len(report.histories))
	}
	got := report.histories[0]
	if got.DownloadedOffset != 150 || got.UploadedOffset != 300 {
		t.Fatalf("raw offsets=%d/%d, want 150/300", got.DownloadedOffset, got.UploadedOffset)
	}
}
