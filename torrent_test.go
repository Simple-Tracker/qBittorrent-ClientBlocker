package main

import "testing"

func TestCheckTorrent(t *testing.T) {
	oldConfig := *config
	oldClient := currentClient
	defer func() {
		tmpConf := oldConfig
	config = &tmpConf
		currentClient = oldClient
	}()

	t.Run("EmptyHash", func(t *testing.T) {
		tmpConf := oldConfig
	config = &tmpConf
		status, peers := CheckTorrent(&Torrent{Hash: ""})
		if status != -1 || peers != nil {
			t.Fatalf("status=%d peers=%v, want -1 nil", status, peers)
		}
	})

	t.Run("IgnorePrivateTracker", func(t *testing.T) {
		tmpConf := oldConfig
	config = &tmpConf
		config.IgnorePTTorrent = true
		status, _ := CheckTorrent(&Torrent{Hash: "abc", Tracker: "Private"})
		if status != -4 {
			t.Fatalf("status=%d, want -4", status)
		}
	})

	t.Run("IgnorePasskeyTracker", func(t *testing.T) {
		tmpConf := oldConfig
	config = &tmpConf
		config.IgnorePTTorrent = true
		status, _ := CheckTorrent(&Torrent{Hash: "abc", Tracker: "https://tracker/announce?passkey=123"})
		if status != -4 {
			t.Fatalf("status=%d, want -4", status)
		}
	})

	t.Run("IgnoreRandomTokenTracker", func(t *testing.T) {
		tmpConf := oldConfig
	config = &tmpConf
		config.IgnorePTTorrent = true
		status, _ := CheckTorrent(&Torrent{Hash: "abc", Tracker: "https://tracker/announce?x=0123456789abcdef0123456789abcdef"})
		if status != -4 {
			t.Fatalf("status=%d, want -4", status)
		}
	})

	t.Run("IgnoreNoLeechers", func(t *testing.T) {
		tmpConf := oldConfig
	config = &tmpConf
		config.IgnoreNoLeechersTorrent = true
		status, _ := CheckTorrent(&Torrent{Hash: "abc", LeechCount: 0})
		if status != -2 {
			t.Fatalf("status=%d, want -2", status)
		}
	})

	t.Run("UseEmbeddedPeers", func(t *testing.T) {
		tmpConf := oldConfig
	config = &tmpConf
		embedded := []*Peer{{IP: "1.1.1.1", Port: 6881}}
		status, peers := CheckTorrent(&Torrent{Hash: "abc", Peers: embedded})
		if status != 0 {
			t.Fatalf("status=%d, want 0", status)
		}
		if len(peers) != 1 || peers[0].IP != "1.1.1.1" {
			t.Fatalf("unexpected peers: %#v", peers)
		}
	})

	t.Run("NoClientAndNoPeers", func(t *testing.T) {
		tmpConf := oldConfig
	config = &tmpConf
		currentClient = nil
		status, peers := CheckTorrent(&Torrent{Hash: "abc"})
		if status != -3 || peers != nil {
			t.Fatalf("status=%d peers=%v, want -3 nil", status, peers)
		}
	})
}

func TestProcessTorrentStatusCounters(t *testing.T) {
	oldConfig := *config
	defer func() { 
		tmpConf := oldConfig
		config = &tmpConf 
	}()
	tmpConf := oldConfig
	config = &tmpConf
	config.SleepTime = 0
	config.IgnoreNoLeechersTorrent = true

	emptyHashCount := 0
	noLeechersCount := 0
	badTorrentInfoCount := 0
	ptTorrentCount := 0
	blockCount := 0
	ipBlockCount := 0
	badPeersCount := 0
	emptyPeersCount := 0

	ProcessTorrent(&Torrent{Hash: ""}, &emptyHashCount, &noLeechersCount, &badTorrentInfoCount, &ptTorrentCount, &blockCount, &ipBlockCount, &badPeersCount, &emptyPeersCount)
	ProcessTorrent(&Torrent{Hash: "abc", LeechCount: 0}, &emptyHashCount, &noLeechersCount, &badTorrentInfoCount, &ptTorrentCount, &blockCount, &ipBlockCount, &badPeersCount, &emptyPeersCount)

	if emptyHashCount != 1 {
		t.Fatalf("emptyHashCount=%d, want 1", emptyHashCount)
	}
	if noLeechersCount != 1 {
		t.Fatalf("noLeechersCount=%d, want 1", noLeechersCount)
	}
}
