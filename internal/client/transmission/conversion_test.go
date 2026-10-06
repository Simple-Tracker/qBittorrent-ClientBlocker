package transmission

import "testing"

func TestTorrentFromResponse(t *testing.T) {
	torrent := TorrentFromResponse(TorrentRecord{
		InfoHash:  "hash-a",
		TotalSize: 12345,
		Private:   true,
		Peers: []PeerRecord{
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
