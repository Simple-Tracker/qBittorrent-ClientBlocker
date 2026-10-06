package app

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/qbittorrent"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/transmission"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/stats"
)

type coverageRoundTripFunc func(*http.Request) (*http.Response, error)

func (f coverageRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func CoverageResponse(code int, body string, headers map[string]string) *http.Response {
	h := make(http.Header)
	for key, value := range headers {
		h.Set(key, value)
	}
	return &http.Response{StatusCode: code, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}

type coverageClient struct {
	clientType string
	detect     bool
	login      bool
	normalBan  int
	shadowBan  int
}

func (c *coverageClient) GetClientType() string { return c.clientType }
func (c *coverageClient) ConfigPath() string    { return "test.conf" }
func (c *coverageClient) SetURL() bool          { return true }
func (c *coverageClient) Login() bool           { return c.login }
func (c *coverageClient) Detect() bool          { return c.detect }
func (c *coverageClient) FetchTorrents() ([]*Torrent, error) {
	return []*Torrent{{Hash: "hash"}}, nil
}
func (c *coverageClient) FetchTorrentPeers(*Torrent) ([]*Peer, error) {
	return []*Peer{{IP: "192.0.2.1"}}, nil
}
func (c *coverageClient) SubmitBlockPeer(map[string]client.BanTarget) bool {
	c.normalBan++
	return true
}
func (c *coverageClient) SubmitShadowBanPeer(map[string]client.BanTarget) bool {
	c.shadowBan++
	return true
}

func TestCheckUpdateReleaseSelectionAndFailures(t *testing.T) {
	oldConfig := *config
	oldClient := httpClientExternal
	oldVersion := programVersion
	oldTimestamp := currentTimestamp
	oldLastTimestamp := lastCheckUpdateTimestamp
	oldRelease := lastCheckUpdateVer
	oldBeta := lastCheckUpdateBetaVer
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		httpClientExternal = oldClient
		programVersion = oldVersion
		currentTimestamp = oldTimestamp
		lastCheckUpdateTimestamp = oldLastTimestamp
		lastCheckUpdateVer = oldRelease
		lastCheckUpdateBetaVer = oldBeta
	})

	testConfig := oldConfig
	testConfig.CheckUpdate = true
	config = &testConfig
	currentTimestamp = 200000
	lastCheckUpdateTimestamp = 0
	programVersion = "3.8b8"
	httpClientExternal = http.Client{Transport: coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return CoverageResponse(200, `[
			{"tag_name":"","prerelease":false},
			{"tag_name":"3.8b9","body":"beta\rnotes","prerelease":true},
			{"tag_name":"3.9","body":"stable\rnotes","prerelease":false}
		]`, nil), nil
	})}
	CheckUpdate()
	if lastCheckUpdateVer != "3.9" || lastCheckUpdateBetaVer != "3.8b9" {
		t.Fatalf("selected releases stable=%q beta=%q", lastCheckUpdateVer, lastCheckUpdateBetaVer)
	}
	CheckUpdate() // 检查任务间隔限制.

	for _, version := range []string{"Unknown", "3.8 (Nightly)", "3.8-test"} {
		programVersion = version
		currentTimestamp += 100000
		lastCheckUpdateTimestamp = 0
		CheckUpdate()
	}

	programVersion = "3.8"
	for _, response := range []struct {
		code int
		body string
	}{{304, ""}, {500, ""}, {200, "{"}, {200, `[{"tag_name":"3.8","prerelease":false}]`}} {
		currentTimestamp += 100000
		lastCheckUpdateTimestamp = 0
		httpClientExternal = http.Client{Transport: coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return CoverageResponse(response.code, response.body, nil), nil
		})}
		CheckUpdate()
	}
}

func TestClientDispatchAndForcedDetection(t *testing.T) {
	oldConfig := *config
	oldClient := currentClient
	oldType := currentClientType
	oldHTTPClient := httpClient
	oldExternal := httpClientExternal
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		currentClient = oldClient
		currentClientType = oldType
		httpClient = oldHTTPClient
		httpClientExternal = oldExternal
	})

	stub := &coverageClient{clientType: "qBittorrent", login: true}
	currentClient = stub
	currentClientType = "qBittorrent"
	testConfig := oldConfig
	testConfig.UseShadowBan = true
	config = &testConfig
	if !Login() {
		t.Fatal("Login did not dispatch")
	}
	if torrents, err := FetchTorrents(); err != nil || len(torrents) != 1 {
		t.Fatalf("FetchTorrents=%v, %v", torrents, err)
	}
	if peers, err := FetchTorrentPeers(&Torrent{}); err != nil || len(peers) != 1 {
		t.Fatalf("FetchTorrentPeers=%v, %v", peers, err)
	}
	if !SubmitBlockPeer(map[string]BlockPeerInfoStruct{"192.0.2.1": {}}) || stub.shadowBan != 1 {
		t.Fatal("shadow ban did not dispatch")
	}
	config.UseShadowBan = false
	if !SubmitBlockPeer(map[string]BlockPeerInfoStruct{"192.0.2.1": {}}) || stub.normalBan != 1 {
		t.Fatal("normal ban did not dispatch")
	}
	currentClientType = "Transmission"
	if TestShadowBanAPI() != 0 {
		t.Fatal("non-qB client should report silent unsupported")
	}
	currentClient = nil
	if Login() || SubmitBlockPeer(map[string]BlockPeerInfoStruct{}) {
		t.Fatal("nil client should fail dispatch")
	}
	if torrents, _ := FetchTorrents(); torrents != nil {
		t.Fatal("nil client returned torrents")
	}
	if peers, _ := FetchTorrentPeers(&Torrent{}); peers != nil {
		t.Fatal("nil client returned peers")
	}

	for _, clientType := range []string{"qBittorrent", "Transmission", "BitComet", "unsupported"} {
		currentClient = nil
		currentClientType = ""
		testConfig = oldConfig
		testConfig.ClientType = clientType
		testConfig.ClientURL = "http://client.invalid"
		config = &testConfig
		httpClient = http.Client{Transport: coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return CoverageResponse(404, "", nil), nil
		})}
		httpClientExternal = httpClient
		if !DetectClient() {
			t.Fatalf("forced detection failed for %q", clientType)
		}
		if clientType == "unsupported" && currentClient != nil {
			t.Fatal("unsupported forced client unexpectedly created")
		}
	}
	currentClient = nil
	config.ClientType = ""
	if DetectClient() {
		t.Fatal("detection unexpectedly succeeded")
	}
}

func TestConfigInitialisationAndRemoteLists(t *testing.T) {
	oldConfig := *config
	oldTransport := httpTransport
	oldClient := httpClient
	oldExternal := httpClientExternal
	oldTimestamp := currentTimestamp
	oldBlockFetch := blockListURLLastFetch
	oldIPFetch := ipBlockListURLLastFetch
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		httpTransport = oldTransport
		httpClient = oldClient
		httpClientExternal = oldExternal
		currentTimestamp = oldTimestamp
		blockListURLLastFetch = oldBlockFetch
		ipBlockListURLLastFetch = oldIPFetch
		EraseSyncMap(&ruleStore.BlockList)
		EraseSyncMap(&ruleStore.IPBlockList)
	})

	testConfig := oldConfig
	testConfig.Interval = 0
	testConfig.RuleCachePath = ""
	testConfig.Timeout = 0
	testConfig.ClientURL = "http://client.invalid///"
	testConfig.Proxy = "None"
	testConfig.LongConnection = true
	testConfig.SkipCertVerification = true
	testConfig.BlockList = []string{"Agent.*", "[", "# ignored"}
	testConfig.IPBlockList = []string{"192.0.2.1", "bad", ""}
	config = &testConfig
	httpTransport = oldTransport.Clone()
	InitConfig()
	if config.Interval != 1 || config.Timeout != 1 || config.ClientURL != "http://client.invalid" {
		t.Fatalf("normalised config: %#v", config)
	}
	if httpTransport.DisableKeepAlives || !httpTransport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("transport settings not applied")
	}
	if _, ok := ruleStore.BlockList.Load("Agent.*"); !ok {
		t.Fatal("block list was not compiled")
	}
	if _, ok := ruleStore.IPBlockList.Load("192.0.2.1"); !ok {
		t.Fatal("IP block list was not compiled")
	}

	currentTimestamp = 1000
	blockListURLLastFetch = 0
	ipBlockListURLLastFetch = 0
	config.UpdateInterval = 10
	config.BlockListURL = []string{"http://lists/plain", "http://lists/json", "http://lists/invalid", "http://lists/missing"}
	config.IPBlockListURL = []string{"http://ips/plain", "http://ips/json", "http://ips/invalid", "http://ips/cached"}
	httpClientExternal = http.Client{Transport: coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/plain":
			return CoverageResponse(200, "PlainAgent\n", map[string]string{"Content-Type": "text/plain"}), nil
		case "/json":
			if r.URL.Host == "ips" {
				return CoverageResponse(200, `["198.51.100.0/24"]`, map[string]string{"Content-Type": "application/json"}), nil
			}
			return CoverageResponse(200, `["JsonAgent"]`, map[string]string{"Content-Type": "application/json; charset=utf-8"}), nil
		case "/invalid":
			return CoverageResponse(200, "{", map[string]string{"Content-Type": "application/json"}), nil
		case "/cached":
			return CoverageResponse(304, "", nil), nil
		default:
			return nil, errors.New("unavailable")
		}
	})}
	if !SetBlockListFromURL() || !SetIPBlockListFromURL() {
		t.Fatal("remote list update failed")
	}
	if _, ok := ruleStore.BlockList.Load("PlainAgent"); !ok {
		t.Fatal("plain remote block list missing")
	}
	if _, ok := ruleStore.IPBlockList.Load("198.51.100.0/24"); !ok {
		t.Fatal("JSON remote IP list missing")
	}
	SetBlockListFromURL()
	SetIPBlockListFromURL()

	if got := FormatConfigValueForLog("SomeToken", "secret"); got != "[REDACTED]" {
		t.Fatalf("token was not redacted: %v", got)
	}
	if got := FormatConfigValueForLog("Secrets", []string{}); len(got.([]string)) != 0 {
		t.Fatalf("empty secrets changed: %v", got)
	}
}

func TestServerRoutesUtilitiesAndLanguage(t *testing.T) {
	oldConfig := *config
	oldType := currentClientType
	oldClient := currentClient
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		currentClientType = oldType
		currentClient = oldClient
	})

	testConfig := oldConfig
	testConfig.WebUI = false
	config = &testConfig
	handler := &httpServerHandler{}
	for _, tc := range []struct {
		method string
		path   string
		code   int
	}{{http.MethodPost, "/", 405}, {http.MethodGet, "/missing", 404}} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))
		if recorder.Code != tc.code {
			t.Fatalf("%s %s status=%d", tc.method, tc.path, recorder.Code)
		}
	}
	currentClientType = "Transmission"
	currentClient = transmission.New(client.Services{Submit: func(string, any, bool, bool, *map[string]string) (int, http.Header, []byte) { return 200, nil, nil }})
	currentClient.SubmitBlockPeer(map[string]client.BanTarget{"192.0.2.9": {}})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ipfilter.dat?x=1", nil))
	if recorder.Code != 200 || recorder.Body.String() != "192.0.2.9" {
		t.Fatalf("Transmission route=%d %q", recorder.Code, recorder.Body.String())
	}

	listener, err := CreateListener("127.0.0.1:0")
	if err != nil {
		t.Fatalf("create IPv4 listener: %v", err)
	}
	_ = listener.Close()
	if IsUnix(`C:\\test`) || !IsUnix("/tmp/test") || !IsIPv6("2001:db8::1") {
		t.Fatal("platform/address helpers failed")
	}
	if !CheckPrivateIP("127.0.0.1") || CheckPrivateIP("bad") {
		t.Fatal("private IP helper failed")
	}
	if ParseIPCIDR("192.0.2.1") == nil || ParseIPCIDR("bad") != nil {
		t.Fatal("CIDR parser failed")
	}
	config.BanIPCIDR = "/24"
	if ParseIPCIDRByConfig("192.0.2.1") == nil || ProcessIP("::FFFF:192.0.2.1") != "192.0.2.1" {
		t.Fatal("configured IP processing failed")
	}
	if count, value := GenIPFilter(1, map[string]BlockPeerInfoStruct{"192.0.2.1": {}, "2001:db8::1": {}}); count != 3 || !strings.Contains(value, "::ffff:192.0.2.1/128") {
		t.Fatalf("type-1 filter count=%d value=%q", count, value)
	}
	if count, value := GenIPFilter(2, map[string]BlockPeerInfoStruct{"192.0.2.1": {}}); count != 2 || !strings.Contains(value, "000") {
		t.Fatalf("type-2 filter count=%d value=%q", count, value)
	}
	if count, value := GenIPFilter(0, nil); count != 0 || value != "" {
		t.Fatal("invalid filter type returned data")
	}

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	oldWorkingDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDir) })
	if errText := SaveIPFilter("filter"); errText != "" {
		t.Fatal(errText)
	}
	if !DeleteIPFilter() || DeleteIPFilter() {
		t.Fatal("IP filter deletion semantics failed")
	}
	if ok, out, errText := ExecCommand("/bin/echo covered"); !ok || !strings.Contains(out, "covered") || errText != "" {
		t.Fatalf("ExecCommand success=%t out=%q err=%q", ok, out, errText)
	}
	if ok, _, _ := ExecCommand("/path/that/does/not/exist"); ok {
		t.Fatal("missing command succeeded")
	}

}

func TestSyncSchedulingAndCrashWrapper(t *testing.T) {
	oldConfig := *config
	oldRules := syncServer_CompiledRules
	oldSyncConfig := syncServer_syncConfig
	oldLastSync := atomic.LoadInt64(&syncServer_lastSync)
	oldSubmitting := syncServer_isSubmiting.Load()
	oldLogPath := config.LogPath
	t.Cleanup(func() {
		restored := oldConfig
		restored.LogPath = oldLogPath
		config = &restored
		syncServer_CompiledRules = oldRules
		syncServer_syncConfig = oldSyncConfig
		atomic.StoreInt64(&syncServer_lastSync, oldLastSync)
		syncServer_isSubmiting.Store(oldSubmitting)
	})

	testConfig := oldConfig
	testConfig.SyncServerURL = ""
	testConfig.LogPath = t.TempDir()
	config = &testConfig
	syncServer_CompiledRules = []SyncServer_RuleStruct{{Net: ParseIPCIDR("192.0.2.0/24")}}
	syncServer_syncConfig = &SyncServer_ConfigStruct{Interval: 1, BlockIPRule: map[string][]string{"x": {"192.0.2.0/24"}}}
	if !SyncWithServer() || len(syncServer_CompiledRules) != 0 || syncServer_syncConfig.Interval != 60 {
		t.Fatal("disabled sync did not reset state")
	}
	if !SyncWithServer_FullSubmit("invalid") && syncServer_isSubmiting.Load() {
		t.Fatal("full submit did not clear in-progress state")
	}

	done := appLogger.GoWithCrashLog("coverage-panic", func() {
		panic("covered")
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("panic wrapper did not return")
	}
	if _, err := os.Stat(appLogger.CrashLogPath()); err != nil {
		t.Fatal("panic wrapper did not write crash log")
	}

	var m sync.Map
	m.Store("x", 1)
	EraseSyncMap(&m)
	if _, ok := m.Load("x"); ok {
		t.Fatal("sync map was not erased")
	}
	if matched, _ := SyncServer_CheckPeer(net.ParseIP("192.0.2.1")); matched {
		t.Fatal("cleared sync rule still matched")
	}
}

func TestLoadInitConfigAppliesFilesWithoutClient(t *testing.T) {
	oldConfig := *config
	oldConfigFilename := configFilename
	oldAdditionalFilename := additionConfigFilename
	oldConfigLastMod := configLastMod
	oldLastURL := lastURL
	oldLogger := appLogger
	appLogger = NewAppLogger()
	t.Cleanup(func() {
		_ = CloseLogFile()
		restored := oldConfig
		config = &restored
		configFilename = oldConfigFilename
		additionConfigFilename = oldAdditionalFilename
		configLastMod = oldConfigLastMod
		lastURL = oldLastURL
		appLogger = oldLogger
	})

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	configFilename = filepath.Join(dir, "config.json")
	additionConfigFilename = filepath.Join(dir, "additional.json")
	configLastMod = make(map[string]int64)
	lastURL = ""
	content := `{"interval":2,"timeout":2,"clientURL":"","useShadowBan":false,"logToFile":true,"logPath":"` + filepath.ToSlash(filepath.Join(dir, "logs")) + `","blockList":["LoadedAgent"],"ipBlockList":["203.0.113.0/24"]}`
	if err := os.WriteFile(configFilename, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if !LoadInitConfig(true) {
		t.Fatal("LoadInitConfig failed without a client URL")
	}
	if config.Interval != 2 || config.ClientURL != "" {
		t.Fatalf("loaded config=%#v", config)
	}
	if _, ok := ruleStore.BlockList.Load("LoadedAgent"); !ok {
		t.Fatal("loaded block rule was not compiled")
	}
	if _, ok := ruleStore.IPBlockList.Load("203.0.113.0/24"); !ok {
		t.Fatal("loaded IP rule was not compiled")
	}
}

func TestServerLifecycleAndClientHelpers(t *testing.T) {
	oldConfig := *config
	oldType := currentClientType
	oldStatus := Server_Status
	oldListeners := Server_Listeners
	oldReadTimeout := httpServer.ReadTimeout
	oldWriteTimeout := httpServer.WriteTimeout
	oldHandler := httpServer.Handler
	oldURL := config.ClientURL
	t.Cleanup(func() {
		if Server_Status {
			StopServer()
		}
		restored := oldConfig
		restored.ClientURL = oldURL
		config = &restored
		currentClientType = oldType
		Server_Status = oldStatus
		Server_Listeners = oldListeners
		httpServer = http.Server{ReadTimeout: oldReadTimeout, WriteTimeout: oldWriteTimeout, Handler: oldHandler}
	})

	testConfig := oldConfig
	testConfig.WebUI = true
	testConfig.WebUIListen = "127.0.0.1:0"
	config = &testConfig
	currentClientType = ""
	Server_Status = false
	Server_Listeners = nil
	connectionClosed := make(chan struct{}, 1)
	httpServer = http.Server{
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		Handler:      &httpServerHandler{},
		ConnState: func(_ net.Conn, state http.ConnState) {
			if state == http.StateClosed || state == http.StateHijacked {
				select {
				case connectionClosed <- struct{}{}:
				default:
				}
			}
		},
	}
	StartServer()
	if !Server_Status || len(Server_Listeners) != 1 {
		t.Fatalf("server status=%t listeners=%d", Server_Status, len(Server_Listeners))
	}
	serverURL := "http://" + Server_Listeners[0].Addr().String() + "/missing"
	response, err := (&http.Client{Timeout: 2 * time.Second}).Get(serverURL)
	if err != nil {
		t.Fatalf("server readiness request: %v", err)
	}
	_ = response.Body.Close()
	StartServer() // 检查任务已在运行时的保护逻辑.
	StopServer()
	select {
	case <-connectionClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("server connection did not close")
	}
	if Server_Status || len(Server_Listeners) != 0 {
		t.Fatal("server did not stop")
	}
	StopServer() // 检查任务已停止时的保护逻辑.

	tr := transmission.New(ClientServices())
	if tr.ConfigPath() != "" || tr.SubmitShadowBanPeer(nil) {
		t.Fatal("Transmission metadata helpers failed")
	}
	if peers, err := tr.FetchTorrentPeers(&Torrent{Peers: []*Peer{{IP: "192.0.2.1"}}}); err != nil || len(peers) != 1 {
		t.Fatalf("Transmission embedded peers=%v err=%v", peers, err)
	}
	config.ClientURL = ""
	if tr.SetURL() {
		t.Fatal("empty Transmission URL was accepted")
	}
}

func TestAggregateChecksCoverPortUploadAndRelativeRules(t *testing.T) {
	oldConfig := *config
	oldTimestamp := currentTimestamp
	oldIPClean := statistics.State().LastIPClean
	oldTorrentClean := statistics.State().LastTorrentClean
	oldBlockPeers := blockPeerMap
	oldBlockCIDRs := blockCIDRMap
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		currentTimestamp = oldTimestamp
		statistics.State().LastIPClean = oldIPClean
		statistics.State().LastTorrentClean = oldTorrentClean
		blockPeerMap = oldBlockPeers
		blockCIDRMap = oldBlockCIDRs
	})

	testConfig := oldConfig
	testConfig.MaxIPPortCount = 1
	testConfig.IPUploadedCheck = true
	testConfig.IPUpCheckIncrementMB = 1
	testConfig.IPUpCheckInterval = 1
	testConfig.IPUpCheckPerTorrentRatio = 2
	testConfig.TorrentMapCleanInterval = 1
	testConfig.BanByRelativeProgressUploaded = true
	testConfig.BanByRelativePUStartMB = 1
	testConfig.BanByRelativePUStartPercent = 1
	testConfig.BanByRelativePUAntiErrorRatio = 2
	config = &testConfig
	currentTimestamp = 100
	statistics.State().LastIPClean = 0
	statistics.State().LastTorrentClean = 0
	blockPeerMap = make(map[string]BlockPeerInfoStruct)
	blockCIDRMap = make(map[string]BlockCIDRInfoStruct)

	netA := ParseIPCIDR("192.0.2.0/24")
	currentIPs := map[string]stats.IPInfoStruct{
		"192.0.2.1":    {Net: netA, Port: map[int]bool{1: true, 2: true}, TorrentDownloaded: map[string]int64{"a": 1}, TorrentUploaded: map[string]int64{"a": 1}},
		"198.51.100.1": {Port: map[int]bool{3: true}, TorrentDownloaded: map[string]int64{"a": 4}, TorrentUploaded: map[string]int64{"a": 5 * 1024 * 1024}},
		"203.0.113.1":  {Port: map[int]bool{}},
	}
	previousIPs := map[string]stats.IPInfoStruct{
		"seed":         {Port: map[int]bool{1: true}, TorrentUploaded: map[string]int64{}},
		"198.51.100.1": {Port: map[int]bool{3: true}, TorrentUploaded: map[string]int64{"a": 1}},
	}
	savedState := statistics.State()
	statistics.ReplaceState(&stats.State{IPMap: currentIPs, LastIPMap: previousIPs})
	t.Cleanup(func() { statistics.ReplaceState(savedState) })
	if count := statistics.CheckAllIP(); count != 2 {
		t.Fatalf("IP aggregate blocks=%d", count)
	}
	if _, ok := blockPeerMap["192.0.2.1"]; !ok {
		t.Fatal("port-count peer not blocked")
	}
	if _, ok := blockPeerMap["198.51.100.1"]; !ok {
		t.Fatal("upload peer not blocked")
	}

	blockPeerMap = make(map[string]BlockPeerInfoStruct)
	blockCIDRMap = make(map[string]BlockCIDRInfoStruct)
	currentTorrents := map[string]stats.TorrentInfoStruct{
		"too-high": {Size: 100 * 1024 * 1024, Peers: map[string]stats.PeerInfoStruct{
			"203.0.113.10": {Port: map[int]bool{10: true}, Progress: 0.01, Uploaded: 10 * 1024 * 1024},
		}},
		"relative": {Size: 100 * 1024 * 1024, Peers: map[string]stats.PeerInfoStruct{
			"203.0.113.11": {Port: map[int]bool{11: true, 12: true}, Progress: 0.5, Uploaded: 20 * 1024 * 1024, ID: "id", Client: "client"},
		}},
	}
	previousTorrents := map[string]stats.TorrentInfoStruct{
		"too-high": {Size: 100 * 1024 * 1024, Peers: map[string]stats.PeerInfoStruct{
			"203.0.113.10": {Port: map[int]bool{10: true}, Progress: 0.01, Uploaded: 1},
		}},
		"relative": {Size: 100 * 1024 * 1024, Peers: map[string]stats.PeerInfoStruct{
			"203.0.113.11": {Port: map[int]bool{11: true}, Progress: 0.49, Uploaded: 1 * 1024 * 1024},
		}},
	}
	statistics.State().TorrentMap, statistics.State().LastTorrentMap = currentTorrents, previousTorrents
	blocks, ipBlocks := statistics.CheckAllTorrent()
	if ipBlocks != 1 || blocks != 1 {
		t.Fatalf("torrent aggregate blocks=%d ipBlocks=%d", blocks, ipBlocks)
	}
}

type coverageErrorReader struct{}

func (coverageErrorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (coverageErrorReader) Close() error             { return nil }

func TestRequestReadFailuresAndRetryStatuses(t *testing.T) {
	oldConfig := *config
	oldClient := httpClient
	oldExternal := httpClientExternal
	oldCurrent := currentClient
	oldType := currentClientType
	oldCount := fetchFailedCount
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		httpClient = oldClient
		httpClientExternal = oldExternal
		currentClient = oldCurrent
		currentClientType = oldType
		fetchFailedCount = oldCount
	})

	stub := &coverageClient{login: true}
	currentClient = stub
	currentClientType = "qBittorrent"
	testConfig := oldConfig
	testConfig.FetchFailedThreshold = 1
	testConfig.ExecCommand_FetchFailed = "/bin/echo retry"
	config = &testConfig
	httpClientExternal = http.Client{Transport: coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("network failed")
	})}
	if code, _, _ := Fetch("http://request.invalid", false, false, false, nil); code != -2 {
		t.Fatalf("network failure code=%d", code)
	}

	httpClient = http.Client{Transport: coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: coverageErrorReader{}}, nil
	})}
	if code, _, _ := Fetch("http://request.invalid", false, true, false, nil); code != -3 {
		t.Fatalf("fetch read failure code=%d", code)
	}
	if code, _, _ := Submit("http://request.invalid", "x", false, true, nil); code != -3 {
		t.Fatalf("submit read failure code=%d", code)
	}

	for _, code := range []int{403, 409} {
		httpClient = http.Client{Transport: coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return CoverageResponse(code, "", nil), nil
		})}
		Fetch("http://request.invalid", true, true, false, nil)
		Submit("http://request.invalid", "", true, true, nil)
	}
}

func TestProcessPeerCountersAndTaskGuards(t *testing.T) {
	oldConfig := *config
	oldCurrent := currentClient
	oldType := currentClientType
	oldIPMap := statistics.State().IPMap
	oldTorrentMap := statistics.State().TorrentMap
	oldBlockPeerMap := blockPeerMap
	oldBlockCIDRMap := blockCIDRMap
	oldBTNConfig := btnConfig
	oldBTNRules := btnRules
	oldBTNExceptions := btnExceptions
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		currentClient = oldCurrent
		currentClientType = oldType
		statistics.State().IPMap = oldIPMap
		statistics.State().TorrentMap = oldTorrentMap
		blockPeerMap = oldBlockPeerMap
		blockCIDRMap = oldBlockCIDRMap
		btnConfig = oldBTNConfig
		btnRules = oldBTNRules
		btnExceptions = oldBTNExceptions
	})

	testConfig := oldConfig
	testConfig.IgnoreEmptyPeer = true
	testConfig.PortBlockList = []uint32{6882}
	testConfig.MaxIPPortCount = 1
	testConfig.SyncServerURL = "enabled"
	testConfig.ClientURL = ""
	config = &testConfig
	statistics.State().IPMap = make(map[string]stats.IPInfoStruct)
	statistics.State().TorrentMap = make(map[string]stats.TorrentInfoStruct)
	blockPeerMap = make(map[string]BlockPeerInfoStruct)
	blockCIDRMap = make(map[string]BlockCIDRInfoStruct)
	btnConfig = nil
	btnRules = &BTN_RulesStruct{}
	btnExceptions = &BTN_ExceptionStruct{}
	blockCount, ipBlockCount, badPeers, emptyPeers := 0, 0, 0, 0
	peers := []*Peer{
		{IP: "", Port: 1, Client: "client"},
		{IP: "192.0.2.1", Port: 1},
		{IP: "192.0.2.2", Port: 6882, Client: "client"},
		{IP: "192.0.2.3", Port: 6881, Client: "client", DlSpeed: 1, Progress: 0.5},
	}
	for _, peer := range peers {
		ProcessPeer(peer, "hash", 100, &blockCount, &ipBlockCount, &badPeers, &emptyPeers)
	}
	if badPeers != 1 || emptyPeers != 1 || blockCount != 1 {
		t.Fatalf("peer counters block=%d ip=%d bad=%d empty=%d", blockCount, ipBlockCount, badPeers, emptyPeers)
	}
	if _, ok := statistics.State().IPMap["192.0.2.3"]; !ok {
		t.Fatal("accepted peer was not aggregated")
	}
	Task()
	config.ClientURL = "http://client.invalid"
	currentClient = nil
	currentClientType = ""
	Task()
}

func TestWebUIServerRoutingAndLogWriter(t *testing.T) {
	oldConfig := *config
	oldLogger := appLogger
	appLogger = NewAppLogger()
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		appLogger = oldLogger
	})
	testConfig := oldConfig
	testConfig.WebUI = true
	testConfig.WebUIUsername = ""
	testConfig.LogToFile = false
	config = &testConfig
	writer := appLogger
	if n, err := writer.Write([]byte("one\n")); err != nil || n != 4 {
		t.Fatalf("log writer n=%d err=%v", n, err)
	}
	_, _ = writer.Write([]byte("two\n"))
	_, _ = writer.Write([]byte("three\n"))
	if len(appLogger.LegacyLogs()) != 3 {
		t.Fatalf("log buffer size=%d", len(appLogger.LegacyLogs()))
	}
	handler := &httpServerHandler{}
	for _, path := range []string{"/", "/api/status", "/api/peers", "/api/logs"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != 200 {
			t.Fatalf("WebUI route %s status=%d", path, recorder.Code)
		}
	}
}

func TestQBConfigRejection(t *testing.T) {
	oldConfig := *config
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
	})
	dir := t.TempDir()
	oldWorkingDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDir) })

	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "qBittorrent", "qBittorrent.ini")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{
		"WebUI\\Enabled=false\nWebUI\\Address=127.0.0.1\n",
		"WebUI\\Enabled=true\nWebUI\\Address=*\nWebUI\\Port=80\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		accepted := qbittorrent.New(ClientServices()).SetURL()
		if strings.Contains(content, "Enabled=false") && accepted {
			t.Fatal("disabled qB WebUI was accepted")
		}
		if strings.Contains(content, "Enabled=true") && (!accepted || config.ClientURL != "http://127.0.0.1/api") {
			t.Fatalf("wildcard qB URL=%q accepted=%t", config.ClientURL, accepted)
		}
	}
	config.ClientURL = ""
	SetURLFromClient()
}
