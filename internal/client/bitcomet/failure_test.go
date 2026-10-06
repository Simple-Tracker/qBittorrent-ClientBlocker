package bitcomet

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func TestBitCometFailureAndParserBranches(t *testing.T) {
	settings := &client.Settings{URL: "http://bitcomet.test"}
	httpClient := http.Client{}
	httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network failure")
	})}
	legacyClient := newHTTPTestClient(t, settings, &httpClient)
	legacyClient.Version = 1
	if torrents, err := legacyClient.FetchTorrents(); err != nil || torrents != nil {
		t.Fatalf("legacy network failure torrents=%#v err=%v", torrents, err)
	}
	if peers, err := legacyClient.FetchTorrentPeers(&client.Torrent{Hash: "1"}); err != nil || peers != nil {
		t.Fatalf("legacy network failure peers=%#v err=%v", peers, err)
	}
	v2Client := newHTTPTestClient(t, settings, &httpClient)
	v2Client.Version = 2
	if torrents, err := v2Client.FetchTorrents(); err != nil || torrents != nil {
		t.Fatalf("v2 network failure torrents=%#v err=%v", torrents, err)
	}
	if peers, err := v2Client.FetchTorrentPeers(&client.Torrent{Hash: "task"}); err != nil || peers != nil {
		t.Fatalf("v2 network failure peers=%#v err=%v", peers, err)
	}
	if !v2Client.SubmitBlockPeer(map[string]client.BanTarget{"no-task": {}}) {
		t.Fatal("empty BitComet task grouping should succeed")
	}

	httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return CoverageResponse(http.StatusOK, "{", nil), nil
	})}
	if _, err := v2Client.FetchTorrents(); err == nil {
		t.Fatal("malformed BitComet torrent response should fail")
	}
	if _, err := v2Client.FetchTorrentPeers(&client.Torrent{Hash: "task"}); err == nil {
		t.Fatal("malformed BitComet peer response should fail")
	}

	httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return CoverageResponse(http.StatusInternalServerError, "failed", nil), nil
	})}
	if v2Client.SubmitBlockPeer(map[string]client.BanTarget{"192.0.2.1": {TaskIDs: []string{"task"}}}) {
		t.Fatal("failed BitComet ban request unexpectedly succeeded")
	}
	if legacyClient.SubmitBlockPeer(nil) || legacyClient.SubmitShadowBanPeer(nil) {
		t.Fatal("legacy BitComet should not support ban submission")
	}

	httpClient = http.Client{Transport: coverageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.Contains(request.URL.Path, "api_v2") {
			return CoverageResponse(http.StatusNotFound, "", nil), nil
		}
		return CoverageResponse(http.StatusUnauthorized, "", map[string]string{"WWW-Authenticate": `Basic realm="BitComet"`}), nil
	})}
	detectedLegacy := newHTTPTestClient(t, settings, &httpClient)
	if !detectedLegacy.Detect() || detectedLegacy.Version != 1 {
		t.Fatalf("legacy BitComet detection version=%d", detectedLegacy.Version)
	}
	if detectedLegacy.Login() {
		t.Fatal("HTTP 401 BitComet login unexpectedly succeeded")
	}

	for input, want := range map[string]int64{"": 0, "1 EB": 1 << 60, "1 PB": 1 << 50, "1 TB": 1 << 40, "1 GB": 1 << 30} {
		if got := ParseSize(input); got != want {
			t.Fatalf("ParseSize(%q)=%d want %d", input, got, want)
		}
	}
	if ParseSpeed("") != 0 || ParsePercent("1") != -1 {
		t.Fatal("empty/short BitComet values were not rejected")
	}
	if _, code := ParseIP("missing-port"); code != -2 {
		t.Fatalf("missing BitComet port code=%d", code)
	}
}
