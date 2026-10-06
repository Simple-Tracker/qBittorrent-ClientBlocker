package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	clientapi "github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/qbittorrent"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/transmission"
)

func TestClientHotSwitchKeepsInstanceAndTypeTogether(t *testing.T) {
	oldClient, oldType := CurrentClientSnapshot()
	oldConfig := ConfigSnapshot()
	t.Cleanup(func() { SetCurrentClient(oldClient, oldType); ReplaceConfig(oldConfig) })
	UpdateConfig(func(cfg *ConfigStruct) { cfg.WebUI = false })
	tr := transmission.New(clientapi.Services{Submit: func(string, any, bool, bool, *map[string]string) (int, http.Header, []byte) {
		return http.StatusOK, nil, nil
	}})
	tr.SetSessionToken("transmission-token")
	tr.SubmitBlockPeer(map[string]clientapi.BanTarget{"192.0.2.1": {}})
	qb := qbittorrent.New(clientapi.Services{})
	SetCurrentClient(tr, "Transmission")

	start := make(chan struct{})
	var done sync.WaitGroup
	done.Add(2)
	go func() {
		defer done.Done()
		<-start
		for i := 0; i < 1000; i++ {
			SetCurrentClient(qb, "qBittorrent")
			SetCurrentClient(tr, "Transmission")
		}
	}()
	go func() {
		defer done.Done()
		<-start
		handler := &httpServerHandler{}
		for i := 0; i < 1000; i++ {
			instance, name := CurrentClientSnapshot()
			if instance.GetClientType() != name {
				t.Errorf("mixed client snapshot: instance=%s type=%s", instance.GetClientType(), name)
				return
			}
			request := NewRequest(false, "http://client.test/rpc", nil, true, false, nil)
			if token := request.Header.Get("X-Transmission-Session-Id"); token != "" && token != "transmission-token" {
				t.Errorf("unexpected session token %q", token)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ipfilter.dat", nil))
			if recorder.Code == http.StatusOK {
				if recorder.Body.String() != "192.0.2.1" {
					t.Errorf("wrong client blocklist %q", recorder.Body.String())
				}
			} else if recorder.Code != http.StatusNotFound {
				t.Errorf("unexpected route status %d", recorder.Code)
			}
		}
	}()
	close(start)
	done.Wait()
}

func TestTransmissionLate409UpdatesRequestOwner(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			oldClient, oldType := CurrentClientSnapshot()
			oldHTTP := httpClient
			t.Cleanup(func() { SetCurrentClient(oldClient, oldType); httpClient = oldHTTP })
			first, second := transmission.New(clientapi.Services{}), transmission.New(clientapi.Services{})
			first.SetSessionToken("first-token")
			second.SetSessionToken("second-token")
			SetCurrentClient(first, "Transmission")
			started, release := make(chan struct{}), make(chan struct{})
			httpClient = http.Client{Transport: coverageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				if token := request.Header.Get("X-Transmission-Session-Id"); token != "first-token" {
					t.Errorf("request used token %q", token)
				}
				close(started)
				<-release
				return &http.Response{StatusCode: http.StatusConflict,
					Header: http.Header{"X-Transmission-Session-Id": []string{"renewed-first-token"}},
					Body:   io.NopCloser(strings.NewReader(""))}, nil
			})}
			done := make(chan struct{})
			go func() {
				defer close(done)
				var status int
				if method == http.MethodGet {
					status, _, _ = Fetch("http://client.test/rpc", false, true, false, nil)
				} else {
					status, _, _ = Submit("http://client.test/rpc", "{}", false, true, nil)
				}
				if status != http.StatusConflict {
					t.Errorf("request status=%d, want 409", status)
				}
			}()
			<-started
			SetCurrentClient(second, "Transmission")
			close(release)
			<-done
			if first.SessionToken() != "renewed-first-token" || second.SessionToken() != "second-token" {
				t.Fatalf("late response changed wrong session: old=%q new=%q", first.SessionToken(), second.SessionToken())
			}
		})
	}
}
