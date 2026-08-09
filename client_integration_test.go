package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	client := &QBClient{}
	if client.GetClientType() != "qBittorrent" || !client.Detect() || !client.Login() {
		t.Fatal("qBittorrent detection/login workflow failed")
	}
	if !QB_TestShadowBanAPI() {
		t.Fatal("shadow-ban capability detection failed")
	}
	peers := map[string]BlockPeerInfoStruct{
		"192.0.2.1":   {Port: map[int]bool{-1: true}},
		"2001:db8::1": {Port: map[int]bool{6881: true}},
	}
	if !client.SubmitShadowBanPeer(peers) {
		t.Fatal("shadow-ban submission failed")
	}
	for _, expected := range []string{"192.0.2.1:1", "[::ffff:192.0.2.1]:1", "[2001:db8::1]:6881"} {
		if !strings.Contains(shadowBanForm, expected) {
			t.Fatalf("shadow-ban form %q does not contain %q", shadowBanForm, expected)
		}
	}
}

func TestTransmissionClientHTTPWorkflow(t *testing.T) {
	oldToken := Tr_csrfToken
	oldFilter := Tr_ipfilterStr
	t.Cleanup(func() {
		Tr_csrfToken = oldToken
		Tr_ipfilterStr = oldFilter
	})
	var methods []string
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request Tr_RequestStruct
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

	client := &TRClient{}
	Tr_SetCSRFToken("test-token")
	if client.GetClientType() != "Transmission" || !client.Detect() || !client.Login() {
		t.Fatal("Transmission detection/login workflow failed")
	}
	torrents, err := client.FetchTorrents()
	if err != nil || len(torrents) != 1 || len(torrents[0].Peers) != 1 {
		t.Fatalf("Transmission torrents=%#v err=%v", torrents, err)
	}
	if !client.SubmitBlockPeer(map[string]BlockPeerInfoStruct{"192.0.2.2": {Port: map[int]bool{51413: true}}}) {
		t.Fatal("Transmission block submission failed")
	}
	if Tr_ipfilterStr != "192.0.2.2" || !strings.Contains(strings.Join(methods, ","), "blocklist-update") {
		t.Fatalf("Transmission filter=%q methods=%v", Tr_ipfilterStr, methods)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://example.com/ipfilter.dat", nil)
	request.RequestURI = "/ipfilter.dat"
	if !Tr_ProcessHTTP(recorder, request) || recorder.Body.String() != "192.0.2.2" {
		t.Fatalf("Transmission ipfilter response=%q", recorder.Body.String())
	}
}

func TestBitCometV2HTTPWorkflow(t *testing.T) {
	var submitted BC_v2_BanParams
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

	client := &BCClient{}
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
	if !client.SubmitBlockPeer(map[string]BlockPeerInfoStruct{
		"192.0.2.3": {InfoHash: "task-a", Port: map[int]bool{6881: true}},
	}) {
		t.Fatal("BitComet ban submission failed")
	}
	if submitted.TaskID != "task-a" || submitted.BanTime != "ban_ip_forever" || len(submitted.IPList) != 1 || submitted.IPList[0] != "192.0.2.3" {
		t.Fatalf("unexpected BitComet ban payload: %#v", submitted)
	}
}

func TestBitCometParsers(t *testing.T) {
	if got := BC_ParseTorrentLink("/panel/task_detail?id=42&x=1"); got != 42 {
		t.Fatalf("torrent id=%d", got)
	}
	if got := BC_ParseSize("1.5 MB"); got != 1572864 {
		t.Fatalf("size=%d", got)
	}
	if got := BC_ParseSpeed("2 KB/s"); got != 2048 {
		t.Fatalf("speed=%d", got)
	}
	if got := BC_ParsePercent("25%"); got != 25 {
		t.Fatalf("percent=%f", got)
	}
	if ip, port := BC_ParseIP("[2001:db8::1]:6881"); ip != "[2001:db8::1]" || port != 6881 {
		t.Fatalf("IP=%q port=%d", ip, port)
	}
	if BC_ParseTorrentLink("/panel/task_detail") != -2 || BC_ParseTorrentLink("/panel/task_detail?id=bad") != -3 {
		t.Fatal("invalid torrent links were accepted")
	}
	for _, invalid := range []string{"bad", "1 XB", "x MB"} {
		if BC_ParseSize(invalid) >= 0 {
			t.Fatalf("invalid size %q was accepted", invalid)
		}
	}
	if BC_ParseSpeed("1 KB") >= 0 || BC_ParsePercent("bad%") >= 0 {
		t.Fatal("invalid speed or percentage was accepted")
	}
	if ip, port := BC_ParseIP("myself"); ip != "" || port != -1 {
		t.Fatalf("myself IP=%q port=%d", ip, port)
	}
	if ip, port := BC_ParseIP("192.0.2.1:bad"); ip != "" || port != -3 {
		t.Fatalf("invalid port IP=%q port=%d", ip, port)
	}
}
