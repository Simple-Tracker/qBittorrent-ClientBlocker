package main

import "testing"

func TestSessionIdentityKeepsIPTorrentAndBanTrafficAligned(t *testing.T) {
	type sample struct {
		id                   string
		raw, total, observed int64
		session              uint64
	}
	for _, tc := range []struct {
		name    string
		samples []sample
	}{
		{"growing counter after replacement", []sample{{"A", 100, 100, 0, 0}, {"B", 150, 250, 150, 1}, {"B", 160, 260, 160, 1}}},
		{"replacement initially unknown", []sample{{"A", 100, 100, 0, 0}, {"B", -1, 100, 0, 1}, {"B", 110, 210, 110, 1}}},
		{"empty identity retains last known", []sample{{"A", 100, 100, 0, 0}, {"", -1, 100, 0, 0}, {"", 110, 110, 10, 0}, {"B", 150, 260, 160, 1}}},
		{"first known identity is not replacement", []sample{{"", 100, 100, 0, 0}, {"A", 150, 150, 50, 0}, {"A", -1, 150, 50, 0}, {"A", 160, 160, 60, 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installCIDRTest(t, "/32", "/128")
			ip, lastKnownID := "203.0.113.10", ""
			for index, sample := range tc.samples {
				currentTimestamp++
				downloaded := sample.raw / 2
				if sample.raw < 0 {
					downloaded = -1
				}
				AddIPInfo(nil, ip, 6881, "hash", downloaded, sample.raw, sample.id)
				AddTorrentInfo("hash", 1000, nil, ip, 6881, .5, downloaded, sample.raw, sample.id, "client")
				if index == 0 {
					AddBlockPeer("test", "initial", ip, 6881, "hash", sample.id, "client", downloaded, sample.raw)
				} else {
					UpdateBlockedPeerTraffic(ip, 6881, "hash", downloaded, sample.raw, sample.id)
				}
				if sample.id != "" {
					lastKnownID = sample.id
				}
				ipInfo := ipMap[ip]
				torrent := torrentMap["hash"].Peers[ip]
				connection := torrent.Connections[6881]
				blocked := blockPeerMap[ip]
				for _, ledger := range []struct {
					name                 string
					downloaded, uploaded int64
				}{
					{"IP", ipInfo.TorrentDownloaded["hash"], ipInfo.TorrentUploaded["hash"]},
					{"torrent", torrent.Downloaded, torrent.Uploaded},
					{"connection", connection.Downloaded, connection.Uploaded},
					{"ban", blocked.Downloaded, blocked.Uploaded},
				} {
					if ledger.downloaded != sample.total/2 || ledger.uploaded != sample.total {
						t.Fatalf("sample=%+v %s totals=%d/%d, want %d/%d", sample, ledger.name, ledger.downloaded, ledger.uploaded, sample.total/2, sample.total)
					}
				}
				if got := ipInfo.TorrentObservedUploaded["hash"]; got != sample.observed {
					t.Fatalf("sample=%+v observed upload=%d, want %d", sample, got, sample.observed)
				}
				if connection.Session != sample.session {
					t.Fatalf("sample=%+v session=%d, want %d", sample, connection.Session, sample.session)
				}
				for _, counter := range []PeerTrafficCounter{ipInfo.TorrentPeers["hash"][6881], connection.Counters, blocked.trafficCounters["hash"][6881]} {
					if counter.PeerID != lastKnownID {
						t.Fatalf("sample=%+v lost last known identity: %+v", sample, counter)
					}
				}
			}
		})
	}
}

func TestSessionIdentityResetsBothDirectionsWhenOneIsUnknown(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	ip := "203.0.113.10"
	for index, sample := range []struct {
		id                                                 string
		downloaded, uploaded, wantDownloaded, wantUploaded int64
	}{
		{"A", 50, 100, 50, 100},
		{"B", -1, 150, 50, 250},
		{"B", 70, -1, 120, 250},
		{"B", 70, 160, 120, 260},
	} {
		currentTimestamp++
		AddIPInfo(nil, ip, 6881, "hash", sample.downloaded, sample.uploaded, sample.id)
		AddTorrentInfo("hash", 1000, nil, ip, 6881, .5, sample.downloaded, sample.uploaded, sample.id, "client")
		if index == 0 {
			AddBlockPeer("test", "initial", ip, 6881, "hash", sample.id, "client", sample.downloaded, sample.uploaded)
		} else {
			UpdateBlockedPeerTraffic(ip, 6881, "hash", sample.downloaded, sample.uploaded, sample.id)
		}
		ipInfo, torrent, blocked := ipMap[ip], torrentMap["hash"].Peers[ip], blockPeerMap[ip]
		for _, pair := range [][2]int64{{ipInfo.TorrentDownloaded["hash"], ipInfo.TorrentUploaded["hash"]}, {torrent.Downloaded, torrent.Uploaded}, {blocked.Downloaded, blocked.Uploaded}} {
			if pair != [2]int64{sample.wantDownloaded, sample.wantUploaded} {
				t.Fatalf("sample=%+v totals=%v", sample, pair)
			}
		}
	}
}

func TestSessionIdentityIsPassedThroughPeerProcessing(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "observed", true: "blocked"}[blocked], func(t *testing.T) {
			installCIDRTest(t, "/32", "/128")
			if blocked {
				UpdateConfig(func(c *ConfigStruct) { c.PortBlockList = []uint32{6881} })
			}
			peer := &Peer{IP: "203.0.113.10", Port: 6881, ID: "A", Client: "client", DlSpeed: 1, Downloaded: 50, Uploaded: 100, Progress: .5}
			processIdleObservation(peer, "hash")
			peer.ID, peer.Downloaded, peer.Uploaded = "B", 75, 150
			processIdleObservation(peer, "hash")
			if blocked {
				if got := blockPeerMap[peer.IP].Uploaded; got != 250 {
					t.Fatalf("blocked processing lost identity: upload=%d", got)
				}
			} else if ipMap[peer.IP].TorrentUploaded["hash"] != 250 || torrentMap["hash"].Peers[peer.IP].Uploaded != 250 {
				t.Fatal("observation processing did not pass identity to both ledgers")
			}
		})
	}
}

func TestSessionIdentityDoesNotInheritAnotherPortsBanMetadata(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	ip := "203.0.113.10"
	AddBlockPeer("test", "first", ip, 6881, "hash", "A", "client", 50, 100)
	AddBlockPeer("test", "second", ip, 6882, "hash", "B", "client", 100, 200)
	AddBlockPeer("test", "repeat", ip, 6881, "hash", "", "client", 55, 110)
	if peer := blockPeerMap[ip]; peer.Downloaded != 155 || peer.Uploaded != 310 || peer.trafficCounters["hash"][6881].PeerID != "A" {
		t.Fatalf("empty observation inherited another port's identity: %+v", peer)
	}
}
