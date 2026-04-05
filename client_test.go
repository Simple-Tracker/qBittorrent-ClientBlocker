package main

import "testing"

func TestIsSupportClient(t *testing.T) {
	oldClient := currentClient
	defer func() { currentClient = oldClient }()

	currentClient = nil
	if IsSupportClient() {
		t.Fatalf("IsSupportClient() should be false when currentClient is nil")
	}

	currentClient = &QBClient{}
	if !IsSupportClient() {
		t.Fatalf("IsSupportClient() should be true when currentClient exists")
	}
}

func TestIsBanPort(t *testing.T) {
	oldClientType := currentClientType
	oldFlag := qB_useNewBanPeersMethod
	defer func() {
		currentClientType = oldClientType
		qB_useNewBanPeersMethod = oldFlag
	}()

	currentClientType = "qBittorrent"
	qB_useNewBanPeersMethod = true
	if !IsBanPort() {
		t.Fatalf("IsBanPort() should be true for qBittorrent with new ban method")
	}

	currentClientType = "Transmission"
	if IsBanPort() {
		t.Fatalf("IsBanPort() should be false for non-qBittorrent client")
	}
}

func TestSubmitBlockPeer_NilInput(t *testing.T) {
	if !SubmitBlockPeer(nil) {
		t.Fatalf("SubmitBlockPeer(nil) should return true")
	}
}

func TestTrTorrentToTorrent(t *testing.T) {
	torrent := trTorrentToTorrent(Tr_TorrentStruct{
		InfoHash:  "hash-a",
		TotalSize: 12345,
		Private:   true,
		Peers: []Tr_PeerStruct{
			{IP: "1.1.1.1", Port: 51413, Client: "peer-a", Progress: 0.5, DlSpeed: 1, UpSpeed: 2, IsUploading: true},
			{IP: "2.2.2.2", Port: 51414, Client: "peer-b", Progress: 0.2, DlSpeed: 3, UpSpeed: 4, IsUploading: false},
		},
	})

	if torrent.Hash != "hash-a" {
		t.Fatalf("Hash=%q, want hash-a", torrent.Hash)
	}
	if torrent.Tracker != "Private" {
		t.Fatalf("Tracker=%q, want Private", torrent.Tracker)
	}
	if torrent.LeechCount != 1 {
		t.Fatalf("LeechCount=%d, want 1", torrent.LeechCount)
	}
	if len(torrent.Peers) != 2 {
		t.Fatalf("len(Peers)=%d, want 2", len(torrent.Peers))
	}
	if torrent.Peers[0].Downloaded != -1 || torrent.Peers[0].Uploaded != -1 {
		t.Fatalf("Transmission peer traffic defaults should remain -1")
	}
}
