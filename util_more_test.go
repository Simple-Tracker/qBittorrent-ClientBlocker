package main

import (
	"reflect"
	"testing"
)

func TestDeepCopyIPMap(t *testing.T) {
	src := map[string]IPInfoStruct{
		"1.2.3.4": {
			Net:             ParseIPCIDR("1.2.3.4/32"),
			Port:            map[int]bool{6881: true},
			TorrentUploaded: map[string]int64{"hash-a": 100},
		},
	}
	dest := map[string]IPInfoStruct{
		"old": {
			Port:            map[int]bool{1: true},
			TorrentUploaded: map[string]int64{"old": 1},
		},
	}

	DeepCopyIPMap(src, dest)

	if _, exists := dest["old"]; exists {
		t.Fatalf("old key should be removed after copy")
	}
	if len(dest) != 1 {
		t.Fatalf("unexpected dest size: got %d", len(dest))
	}

	srcInfo := src["1.2.3.4"]
	srcInfo.Port[7000] = true
	srcInfo.TorrentUploaded["hash-a"] = 200
	src["1.2.3.4"] = srcInfo

	if dest["1.2.3.4"].Port[7000] {
		t.Fatalf("dest map should not share nested Port map")
	}
	if dest["1.2.3.4"].TorrentUploaded["hash-a"] != 100 {
		t.Fatalf("dest map should not share nested TorrentUploaded map")
	}
}

func TestDeepCopyTorrentMap(t *testing.T) {
	src := map[string]TorrentInfoStruct{
		"hash-a": {
			Size: 1000,
			Peers: map[string]PeerInfoStruct{
				"1.2.3.4": {
					Net:      ParseIPCIDR("1.2.3.4/32"),
					Port:     map[int]bool{6881: true},
					Progress: 0.5,
					Uploaded: 500,
				},
			},
		},
	}
	dest := map[string]TorrentInfoStruct{
		"old": {Size: 1, Peers: map[string]PeerInfoStruct{}},
	}

	DeepCopyTorrentMap(src, dest)

	if _, exists := dest["old"]; exists {
		t.Fatalf("old key should be removed after copy")
	}
	if len(dest) != 1 {
		t.Fatalf("unexpected dest size: got %d", len(dest))
	}

	peer := src["hash-a"].Peers["1.2.3.4"]
	peer.Port[7000] = true
	peer.Uploaded = 900
	srcPeers := src["hash-a"].Peers
	srcPeers["1.2.3.4"] = peer
	src["hash-a"] = TorrentInfoStruct{Size: 1000, Peers: srcPeers}

	if dest["hash-a"].Peers["1.2.3.4"].Port[7000] {
		t.Fatalf("dest map should not share nested peer port map")
	}
	if dest["hash-a"].Peers["1.2.3.4"].Uploaded != 500 {
		t.Fatalf("dest map should not share nested peer struct")
	}
}

func TestCompareVersion(t *testing.T) {
	testCases := []struct {
		v1   string
		v2   string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.2.10", "1.2.9", 1},
		{"1.0.0", "1", 0},
		{"1.0.0", "1.0.0.1", -1},
		{"2.0", "1.9.9", 1},
		{"1.10", "1.2.99", 1},
	}

	for _, tc := range testCases {
		got := CompareVersion(tc.v1, tc.v2)
		if got != tc.want {
			t.Fatalf("CompareVersion(%q, %q) = %d, want %d", tc.v1, tc.v2, got, tc.want)
		}
	}
}

func TestParseCommand(t *testing.T) {
	got := ParseCommand(`cmd "arg one" 'arg two' plain`)
	want := []string{"cmd", "arg one", "arg two", "plain"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseCommand() = %#v, want %#v", got, want)
	}
}
