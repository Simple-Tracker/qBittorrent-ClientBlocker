package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/bitcomet"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/qbittorrent"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/transmission"
)

func InstallClientTestServer(t *testing.T, handler http.Handler) {
	t.Helper()
	oldConfig := *config
	oldHTTPClient := httpClient
	oldHTTPClientExternal := httpClientExternal
	server := httptest.NewServer(handler)
	httpClient = *server.Client()
	httpClientExternal = *server.Client()
	testConfig := oldConfig
	testConfig.ClientURL = server.URL
	testConfig.UseBasicAuth = false
	config = &testConfig
	t.Cleanup(func() {
		server.Close()
		restored := oldConfig
		config = &restored
		httpClient = oldHTTPClient
		httpClientExternal = oldHTTPClientExternal
	})
}

func TestQBClientHTTPWorkflow(t *testing.T) {
	var shadowBanForm string
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/app/webapiVersion":
			_, _ = w.Write([]byte("2.11"))
		case "/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/v2/app/preferences":
			_, _ = w.Write([]byte("{\"shadow_ban_enabled\":true}"))
		case "/v2/transfer/shadowbanPeers":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse shadow-ban form: %v", err)
			}
			shadowBanForm = r.PostForm.Get("peers")
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))

	client := qbittorrent.New(ClientServices())
	if client.GetClientType() != "qBittorrent" || !client.Detect() || !client.Login() {
		t.Fatal("qBittorrent detection/login workflow failed")
	}
	if !client.TestShadowBanAPI() {
		t.Fatal("shadow-ban capability detection failed")
	}
	peers := map[string]BlockPeerInfoStruct{
		"192.0.2.1":   {Port: map[int]bool{-1: true}},
		"2001:db8::1": {Port: map[int]bool{6881: true}},
	}
	if !client.SubmitShadowBanPeer(ToClientBans(peers)) {
		t.Fatal("shadow-ban submission failed")
	}
	for _, expected := range []string{"192.0.2.1:1", "[::ffff:192.0.2.1]:1", "[2001:db8::1]:6881"} {
		if !strings.Contains(shadowBanForm, expected) {
			t.Fatalf("shadow-ban form %q does not contain %q", shadowBanForm, expected)
		}
	}
}

func TestTransmissionClientHTTPWorkflow(t *testing.T) {
	oldClient, oldType := currentClient, currentClientType
	t.Cleanup(func() { currentClient, currentClientType = oldClient, oldType })
	var methods []string
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request transmission.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Transmission request: %v", err)
			return
		}
		methods = append(methods, request.Method)
		switch request.Method {
		case "session-get":
			_, _ = w.Write([]byte("{\"result\":\"success\"}"))
		case "torrent-get":
			_, _ = w.Write([]byte("{\"result\":\"success\",\"arguments\":{\"torrents\":[{\"hashString\":\"hash-a\",\"totalSize\":1000,\"private\":false,\"peers\":[{\"address\":\"192.0.2.2\",\"port\":51413,\"clientName\":\"peer-a\",\"progress\":0.5,\"isUploadingTo\":true,\"rateToClient\":1,\"rateToPeer\":2}]}]}}"))
		case "blocklist-update":
			_, _ = w.Write([]byte("{\"result\":\"success\"}"))
		default:
			http.Error(w, "unknown method", http.StatusBadRequest)
		}
	}))

	client := transmission.New(ClientServices())
	client.SetSessionToken("test-token")
	currentClient, currentClientType = client, "Transmission"
	if client.GetClientType() != "Transmission" || !client.Detect() || !client.Login() {
		t.Fatal("Transmission detection/login workflow failed")
	}
	torrents, err := client.FetchTorrents()
	if err != nil || len(torrents) != 1 || len(torrents[0].Peers) != 1 {
		t.Fatalf("Transmission torrents=%#v err=%v", torrents, err)
	}
	if !client.SubmitBlockPeer(ToClientBans(map[string]BlockPeerInfoStruct{"192.0.2.2": {Port: map[int]bool{51413: true}}})) {
		t.Fatal("Transmission block submission failed")
	}
	if !strings.Contains(strings.Join(methods, ","), "blocklist-update") {
		t.Fatalf("Transmission methods=%v", methods)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://example.com/ipfilter.dat", nil)
	request.RequestURI = "/ipfilter.dat"
	if !client.ServeBlocklist(recorder, request) || recorder.Body.String() != "192.0.2.2" {
		t.Fatalf("Transmission ipfilter response=%q", recorder.Body.String())
	}
}

func TestBitCometV2HTTPWorkflow(t *testing.T) {
	var submitted bitcomet.BanParams
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api_v2/task_list/get":
			_, _ = w.Write([]byte("{\"movie_list\":[{\"task_id\":\"task-a\",\"type\":\"BT\",\"total_size\":1000,\"leechers_count\":2},{\"task_id\":\"task-http\",\"type\":\"HTTP\",\"total_size\":2000}]}"))
		case "/panel/":
			_, _ = w.Write([]byte("ok"))
		case "/api/task/peers/get":
			_, _ = w.Write([]byte("{\"peers_connected\":[{\"address\":\"192.0.2.3\",\"remoteport\":6881,\"clienttype\":\"peer-a\",\"progress\":50,\"downrate\":1,\"uprate\":2,\"downsize\":3,\"upsize\":4}]}"))
		case "/api/task/peers/ban_ip":
			if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
				t.Errorf("decode BitComet ban: %v", err)
			}
			_, _ = w.Write([]byte("{\"result\":\"success\"}"))
		default:
			http.NotFound(w, r)
		}
	}))

	client := bitcomet.New(ClientServices())
	if client.GetClientType() != "BitComet" || !client.Detect() || client.Version != 2 || !client.Login() {
		t.Fatal("BitComet v2 detection/login workflow failed")
	}
	torrents, err := client.FetchTorrents()
	if err != nil || len(torrents) != 1 || torrents[0].Hash != "task-a" {
		t.Fatalf("BitComet torrents=%#v err=%v", torrents, err)
	}
	peers, err := client.FetchTorrentPeers(torrents[0])
	if err != nil || len(peers) != 1 || peers[0].Progress != 0.5 {
		t.Fatalf("BitComet peers=%#v err=%v", peers, err)
	}
	if !client.SubmitBlockPeer(ToClientBans(map[string]BlockPeerInfoStruct{
		"192.0.2.3": {InfoHash: "task-a", Port: map[int]bool{6881: true}},
	})) {
		t.Fatal("BitComet ban submission failed")
	}
	if submitted.TaskID != "task-a" || submitted.BanTime != "ban_ip_forever" || len(submitted.IPList) != 1 || submitted.IPList[0] != "192.0.2.3" {
		t.Fatalf("unexpected BitComet ban payload: %#v", submitted)
	}
}

func TestTransmissionSessionBootstrapAndRenewal(t *testing.T) {
	oldClient, oldType := currentClient, currentClientType
	t.Cleanup(func() { currentClient, currentClientType = oldClient, oldType })
	calls := 0
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		wantToken := []string{"", "initial-token", "renewed-token"}[calls-1]
		if got := r.Header.Get("X-Transmission-Session-Id"); got != wantToken {
			t.Errorf("request %d session token=%q, want %q", calls, got, wantToken)
		}
		if calls < 3 {
			w.Header().Set("X-Transmission-Session-Id", []string{"initial-token", "renewed-token"}[calls-1])
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.Write([]byte(`{"result":"success","arguments":{"torrents":[]}}`))
	}))
	c := transmission.New(ClientServices())
	currentClient, currentClientType = c, "Transmission"
	if !c.Login() || c.SessionToken() != "initial-token" {
		t.Fatal("login did not retain the session token from HTTP 409")
	}
	if torrents, err := c.FetchTorrents(); err != nil || torrents != nil || c.SessionToken() != "renewed-token" {
		t.Fatalf("session renewal torrents=%v error=%v token=%q", torrents, err, c.SessionToken())
	}
	if torrents, err := c.FetchTorrents(); err != nil || torrents == nil || len(torrents) != 0 || calls != 3 {
		t.Fatalf("renewed request torrents=%v error=%v calls=%d", torrents, err, calls)
	}
}
