package transmission

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func TestTransmissionFailureBranches(t *testing.T) {
	settings := &client.Settings{}
	httpClient := http.Client{}
	c := newHTTPTestClient(t, settings, &httpClient)
	if c.SetURL() {
		t.Fatal("Transmission URL setup should reject an empty client URL")
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://example.test/not-filter", nil)
	request.RequestURI = "/not-filter"
	if c.ServeBlocklist(recorder, request) {
		t.Fatal("unrelated Transmission HTTP path was handled")
	}
	if c.SubmitShadowBanPeer(nil) {
		t.Fatal("Transmission should not support shadow banning")
	}

	settings.URL = "http://transmission.test/rpc"
	settings.BlocklistURL = "http://sync.test"
	httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return CoverageResponse(http.StatusInternalServerError, "", nil), nil
	})}
	if !c.SetURL() {
		t.Fatal("Transmission URL setup should submit session configuration")
	}
	if c.Detect() {
		t.Fatal("Transmission detection should reject HTTP 500")
	}

	c.SetSessionToken("")
	if c.Login() {
		t.Fatal("Transmission login should fail without a CSRF token")
	}
	if torrents := c.fetchTorrents(); torrents != nil {
		t.Fatalf("empty Transmission response returned %#v", torrents)
	}
	if torrents, err := c.FetchTorrents(); err != nil || torrents != nil {
		t.Fatalf("client empty torrents=%#v err=%v", torrents, err)
	}

	httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return CoverageResponse(http.StatusOK, "{", nil), nil
	})}
	if torrents := c.fetchTorrents(); torrents != nil {
		t.Fatalf("malformed Transmission response returned %#v", torrents)
	}
}
