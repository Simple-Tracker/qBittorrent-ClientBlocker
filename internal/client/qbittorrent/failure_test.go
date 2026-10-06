package qbittorrent

import (
	"net/http"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func TestQBFailureBranchesAndSuffixDetection(t *testing.T) {
	settings := &client.Settings{URL: "http://qb.invalid"}
	mode := "suffix"
	transport := coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if mode == "suffix" && r.URL.Path == "/api/v2/app/webapiVersion" {
			return CoverageResponse(403, "", nil), nil
		}
		switch r.URL.Path {
		case "/api/v2/auth/login":
			switch mode {
			case "login204":
				return CoverageResponse(204, "", nil), nil
			case "loginfail":
				return CoverageResponse(200, "Fails.", nil), nil
			default:
				return CoverageResponse(200, "Other", nil), nil
			}
		case "/api/v2/app/preferences":
			switch mode {
			case "missing-pref":
				return CoverageResponse(200, `{}`, nil), nil
			case "bad-pref":
				return CoverageResponse(200, `{"shadow_ban_enabled":"yes"}`, nil), nil
			case "off-pref":
				return CoverageResponse(200, `{"shadow_ban_enabled":false}`, nil), nil
			case "invalid-pref":
				return CoverageResponse(200, `{`, nil), nil
			}
		case "/api/v2/torrents/info", "/api/v2/sync/torrentPeers":
			return CoverageResponse(200, `{`, nil), nil
		}
		return CoverageResponse(404, "", nil), nil
	})
	httpClient := http.Client{Transport: transport}
	c := newHTTPTestClient(t, settings, &httpClient)
	if !c.Detect() || settings.URL != "http://qb.invalid/api" {
		t.Fatalf("suffix detection URL=%q", settings.URL)
	}
	mode = "login204"
	if !c.Login() {
		t.Fatal("204 login failed")
	}
	for _, loginMode := range []string{"loginfail", "loginother"} {
		mode = loginMode
		if c.Login() {
			t.Fatalf("%s unexpectedly logged in", loginMode)
		}
	}
	for _, preferenceMode := range []string{"missing-pref", "bad-pref", "off-pref", "invalid-pref"} {
		mode = preferenceMode
		if c.TestShadowBanAPI() {
			t.Fatalf("%s unexpectedly enabled shadow ban", preferenceMode)
		}
	}
	if torrents, _ := c.FetchTorrents(); torrents != nil {
		t.Fatal("invalid torrent JSON was accepted")
	}
	if peers, _ := c.FetchTorrentPeers(&client.Torrent{Hash: "x"}); peers != nil {
		t.Fatal("invalid peer JSON was accepted")
	}
	if c.SubmitBlockPeer(map[string]client.BanTarget{}) {
		t.Fatal("failed qB submission returned success")
	}
}
