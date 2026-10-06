package qbittorrent

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func TestQBFailureResponseBranches(t *testing.T) {
	settings := &client.Settings{URL: "http://qb.test/api"}
	httpClient := http.Client{}
	c := newHTTPTestClient(t, settings, &httpClient)
	httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network failure")
	})}
	if c.login() || c.fetchTorrents() != nil || c.fetchTorrentPeers("hash") != nil || c.preferences() != nil {
		t.Fatal("qBittorrent network failures returned data")
	}
	settings.NewBanPeersMethod = false
	if c.submitBlockPeer(map[string]client.BanTarget{"192.0.2.1": {}}) {
		t.Fatal("qBittorrent failed ban request unexpectedly succeeded")
	}
	if c.submitShadowBanPeer(map[string]client.BanTarget{"192.0.2.1": {Ports: map[int]bool{6881: true}}}) {
		t.Fatal("qBittorrent failed shadow-ban request unexpectedly succeeded")
	}
	if !c.submitShadowBanPeer(nil) {
		t.Fatal("empty qBittorrent shadow-ban should succeed")
	}

	for _, body := range []string{"Fails.", "unexpected"} {
		responseBody := body
		httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return CoverageResponse(http.StatusOK, responseBody, nil), nil
		})}
		if c.login() {
			t.Fatalf("qBittorrent login body %q unexpectedly succeeded", body)
		}
	}
	httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return CoverageResponse(http.StatusNoContent, "", nil), nil
	})}
	if !c.login() {
		t.Fatal("qBittorrent HTTP 204 login should succeed")
	}

	httpClient = http.Client{Transport: coverageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api/v2/torrents/info", "/api/v2/sync/torrentPeers", "/api/v2/app/preferences":
			return CoverageResponse(http.StatusOK, "{", nil), nil
		default:
			return CoverageResponse(http.StatusInternalServerError, "", nil), nil
		}
	})}
	if c.fetchTorrents() != nil || c.fetchTorrentPeers("hash") != nil || c.preferences() != nil {
		t.Fatal("malformed qBittorrent JSON returned data")
	}

	for _, preferences := range []string{`{}`, `{"shadow_ban_enabled":false}`, `{"shadow_ban_enabled":"yes"}`} {
		body := preferences
		httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return CoverageResponse(http.StatusOK, body, nil), nil
		})}
		if c.TestShadowBanAPI() {
			t.Fatalf("preferences %s unexpectedly enabled shadow ban", preferences)
		}
	}
	httpClient = http.Client{Transport: coverageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/preferences") {
			return CoverageResponse(http.StatusOK, `{"shadow_ban_enabled":true}`, nil), nil
		}
		return CoverageResponse(http.StatusInternalServerError, "", nil), nil
	})}
	if c.TestShadowBanAPI() {
		t.Fatal("failed qBittorrent shadow-ban probe unexpectedly succeeded")
	}
}
