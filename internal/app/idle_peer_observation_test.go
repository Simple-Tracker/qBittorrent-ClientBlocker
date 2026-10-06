package app

import "testing"

func processIdleObservation(peer *Peer, hash string) (blocks, ipBlocks, bad, empty int) {
	ProcessPeer(peer, hash, 100<<20, &blocks, &ipBlocks, &bad, &empty)
	return
}

func TestIdleKnownPeerRecordsFinalCountersInEachEnabledHistory(t *testing.T) {
	for _, history := range []string{"both", "ip only", "torrent only"} {
		t.Run(history, func(t *testing.T) {
			installCIDRTest(t, "/32", "/128")
			UpdateConfig(func(c *ConfigStruct) {
				if history == "ip only" {
					c.IPUpCheckPerTorrentRatio = 0
				}
				if history == "torrent only" {
					c.MaxIPPortCount, c.IPUpCheckIncrementMB = 0, 0
				}
			})
			peer := &Peer{IP: "203.0.113.10", Port: 6881, ID: "peer", Client: "client", DlSpeed: 1, Downloaded: 50, Uploaded: 100, Progress: .5}
			processIdleObservation(peer, "hash")
			currentTimestamp = 110
			peer.DlSpeed, peer.UpSpeed, peer.Downloaded, peer.Uploaded = 0, 0, 75, 150
			for sample := 0; sample < 2; sample++ {
				if blocks, ipBlocks, bad, empty := processIdleObservation(peer, "hash"); blocks+ipBlocks+bad+empty != 0 {
					t.Fatalf("known idle observation was not recorded: counts=%d/%d/%d/%d", blocks, ipBlocks, bad, empty)
				}
			}
			if history != "torrent only" {
				info := statistics.State().IPMap[peer.IP]
				if info.TorrentDownloaded["hash"] != 75 || info.TorrentUploaded["hash"] != 150 || info.TorrentPeers["hash"][6881].LastSeen != 110 {
					t.Fatalf("IP history missed final counters: %+v", info)
				}
			}
			if history != "ip only" {
				info := statistics.State().TorrentMap["hash"].Peers[peer.IP].Connections[6881]
				if info.Downloaded != 75 || info.Uploaded != 150 || info.FirstSeen != 100 || info.LastSeen != 110 {
					t.Fatalf("torrent history missed final counters: %+v", info)
				}
			}
		})
	}
}

func TestIdleUnseenEndpointDoesNotStartHistory(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	peer := &Peer{IP: "203.0.113.10", Port: 6881, ID: "peer", Client: "client", DlSpeed: 1, Uploaded: 100, Progress: .5}
	processIdleObservation(peer, "hash")
	peer.DlSpeed = 0
	for _, endpoint := range []struct {
		ip, hash string
		port     int
	}{
		{"203.0.113.11", "hash", 6881},
		{"203.0.113.10", "other-hash", 6881},
		{"203.0.113.10", "hash", 6882},
	} {
		peer.IP, peer.Port = endpoint.ip, endpoint.port
		blocks, ipBlocks, bad, empty := processIdleObservation(peer, endpoint.hash)
		if blocks+ipBlocks+bad != 0 || empty != 1 || IsTrackedPeer(endpoint.ip, endpoint.port, endpoint.hash) {
			t.Fatalf("new idle endpoint was admitted: %+v counts=%d/%d/%d/%d", endpoint, blocks, ipBlocks, bad, empty)
		}
	}
}

func TestIdleKnownPeerStillHonorsIgnoreFilters(t *testing.T) {
	for _, filter := range []string{"downloaded", "empty identity", "private torrent"} {
		t.Run(filter, func(t *testing.T) {
			installCIDRTest(t, "/32", "/128")
			peer := &Peer{IP: "203.0.113.10", Port: 6881, ID: "peer", Client: "client", DlSpeed: 1, Uploaded: 100, Progress: .5}
			processIdleObservation(peer, "hash")
			currentTimestamp = 110
			peer.DlSpeed, peer.Uploaded = 0, 150
			UpdateConfig(func(c *ConfigStruct) {
				switch filter {
				case "downloaded":
					c.IgnoreByDownloaded = 1
					peer.Downloaded = 2 << 20
				case "empty identity":
					c.IgnoreEmptyPeer = true
					peer.ID, peer.Client = "", ""
				case "private torrent":
					c.IgnorePTTorrent = true
				}
			})
			if filter == "private torrent" {
				var emptyHash, noLeechers, badTorrent, private, blocks, ipBlocks, badPeers, emptyPeers int
				ProcessTorrent(&Torrent{Hash: "hash", Tracker: "Private", Peers: []*Peer{peer}}, &emptyHash, &noLeechers, &badTorrent, &private, &blocks, &ipBlocks, &badPeers, &emptyPeers)
				if private != 1 {
					t.Fatal("private torrent was not filtered")
				}
			} else {
				_, _, _, empty := processIdleObservation(peer, "hash")
				if empty != 1 {
					t.Fatal("ignored idle peer was not filtered")
				}
			}
			if info := statistics.State().IPMap[peer.IP]; info.TorrentUploaded["hash"] != 100 || info.LastSeen != 100 {
				t.Fatalf("ignored peer changed IP history: %+v", info)
			}
			if info := statistics.State().TorrentMap["hash"].Peers[peer.IP].Connections[6881]; info.Uploaded != 100 || info.LastSeen != 100 {
				t.Fatalf("ignored peer changed torrent history: %+v", info)
			}
		})
	}
}

func TestIdleBlockedPeerUpdatesOnlyBanTraffic(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	peer := &Peer{IP: "203.0.113.10", Port: 6881, ID: "peer", Client: "client", DlSpeed: 1, Downloaded: 50, Uploaded: 100, Progress: .5}
	processIdleObservation(peer, "hash")
	AddBlockPeer("test", "initial", peer.IP, peer.Port, "hash", peer.ID, peer.Client, 50, 100)
	currentTimestamp = 110
	peer.DlSpeed, peer.Downloaded, peer.Uploaded = 0, 75, 150
	for sample := 0; sample < 2; sample++ {
		if blocks, ipBlocks, bad, empty := processIdleObservation(peer, "hash"); blocks+ipBlocks+bad+empty != 0 {
			t.Fatalf("blocked observation was reprocessed: counts=%d/%d/%d/%d", blocks, ipBlocks, bad, empty)
		}
	}
	if blocked := blockPeerMap[peer.IP]; blocked.Downloaded != 75 || blocked.Uploaded != 150 {
		t.Fatalf("blocked final counters were duplicated or lost: %+v", blocked)
	}
	if statistics.State().IPMap[peer.IP].TorrentUploaded["hash"] != 100 || statistics.State().TorrentMap["hash"].Peers[peer.IP].Uploaded != 100 {
		t.Fatal("blocked observation also updated unblocked history")
	}
}
