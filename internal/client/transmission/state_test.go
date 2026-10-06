package transmission

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func TestClientsKeepSessionAndBlocklistStateSeparate(t *testing.T) {
	services := client.Services{Submit: func(string, any, bool, bool, *map[string]string) (int, http.Header, []byte) {
		return http.StatusOK, nil, nil
	}}
	first, second := New(services), New(services)
	first.SetSessionToken("first-token")
	if second.SessionToken() != "" {
		t.Fatal("session token was shared between clients")
	}
	first.SubmitBlockPeer(map[string]client.BanTarget{"192.0.2.1": {}})
	for _, test := range []struct {
		client *Client
		want   string
	}{{first, "192.0.2.1"}, {second, ""}} {
		recorder := httptest.NewRecorder()
		if !test.client.ServeBlocklist(recorder, httptest.NewRequest(http.MethodGet, "/ipfilter.dat", nil)) || recorder.Body.String() != test.want {
			t.Fatalf("blocklist=%q, want %q", recorder.Body.String(), test.want)
		}
	}
}
