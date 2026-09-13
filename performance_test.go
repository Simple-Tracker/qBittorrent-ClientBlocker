package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

const screenshotPeerCount = 13327
const screenshotTimestamp int64 = 1773000000

func MakeScreenshotScalePeers() (map[string]BlockPeerInfoStruct, []string) {
	random := rand.New(rand.NewSource(3809))
	modules := []string{"CheckPeer", "BTN", "SyncServer"}
	reasons := []string{"Bad-Client_Normal", "Bad-Port", "Reputation"}
	clients := []string{"qBittorrent/5.0", "Transmission/4.0", "BitComet/2.15"}
	peers := make(map[string]BlockPeerInfoStruct, screenshotPeerCount)
	peerIPs := make([]string, 0, screenshotPeerCount)

	for len(peers) < screenshotPeerCount {
		peerIP := fmt.Sprintf("%d.%d.%d.%d", 11+random.Intn(212), random.Intn(256), random.Intn(256), 1+random.Intn(254))
		if _, exists := peers[peerIP]; exists {
			continue
		}
		port := 1 + random.Intn(65535)
		index := len(peers)
		peers[peerIP] = BlockPeerInfoStruct{
			Timestamp:  screenshotTimestamp - int64(random.Intn(3600)),
			Module:     modules[random.Intn(len(modules))],
			Reason:     reasons[random.Intn(len(reasons))],
			Port:       map[int]bool{port: true},
			InfoHash:   fmt.Sprintf("%040x", random.Uint64()),
			ID:         "peer-" + strconv.Itoa(index),
			Client:     clients[random.Intn(len(clients))],
			Downloaded: random.Int63n(1 << 34),
			Uploaded:   random.Int63n(1 << 33),
		}
		peerIPs = append(peerIPs, peerIP)
	}

	return peers, peerIPs
}

func InstallScreenshotScalePeers(tb testing.TB, peers map[string]BlockPeerInfoStruct) {
	tb.Helper()
	oldConfig := *config
	oldCurrentClientType := currentClientType
	oldQBMethod := qB_useNewBanPeersMethod
	oldCurrentTimestamp := currentTimestamp
	oldLastCleanTimestamp := lastCleanTimestamp

	blockPeerMapMutex.Lock()
	oldBlockPeerMap := blockPeerMap
	blockPeerMap = peers
	blockPeerMapMutex.Unlock()
	blockCIDRMapMutex.Lock()
	oldBlockCIDRMap := blockCIDRMap
	blockCIDRMap = map[string]BlockCIDRInfoStruct{}
	blockCIDRMapMutex.Unlock()
	webUIPeerSyncMutex.Lock()
	oldCursor := webUIPeerSyncCursor
	oldEvents := webUIPeerSyncEvents
	oldEventStart := webUIPeerSyncEventStart
	webUIPeerSyncCursor = 0
	webUIPeerSyncEvents = nil
	webUIPeerSyncEventStart = 0
	webUIPeerSyncMutex.Unlock()

	testConfig := oldConfig
	testConfig.BanAllPort = false
	testConfig.BanTime = 86400
	testConfig.CleanInterval = 0
	testConfig.Debug = false
	testConfig.ExecCommand_Ban = ""
	testConfig.ExecCommand_Unban = ""
	testConfig.LogToFile = false
	testConfig.WebUI = false
	config = &testConfig
	currentClientType = "qBittorrent"
	qB_useNewBanPeersMethod = true
	currentTimestamp = screenshotTimestamp
	lastCleanTimestamp = 0

	tb.Cleanup(func() {
		restored := oldConfig
		config = &restored
		currentClientType = oldCurrentClientType
		qB_useNewBanPeersMethod = oldQBMethod
		currentTimestamp = oldCurrentTimestamp
		lastCleanTimestamp = oldLastCleanTimestamp
		blockPeerMapMutex.Lock()
		blockPeerMap = oldBlockPeerMap
		blockPeerMapMutex.Unlock()
		blockCIDRMapMutex.Lock()
		blockCIDRMap = oldBlockCIDRMap
		blockCIDRMapMutex.Unlock()
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncCursor = oldCursor
		webUIPeerSyncEvents = oldEvents
		webUIPeerSyncEventStart = oldEventStart
		webUIPeerSyncMutex.Unlock()
	})
}

func MakeScreenshotScaleQBServer(tb testing.TB, peers map[string]BlockPeerInfoStruct, peerIPs []string, torrentCount int) (*httptest.Server, *int64) {
	tb.Helper()
	torrents := make([]qB_TorrentStruct, torrentCount)
	peersByTorrent := make([]map[string]qB_PeerStruct, torrentCount)
	for index := range torrents {
		torrents[index] = qB_TorrentStruct{
			InfoHash:  fmt.Sprintf("%040x", index+1),
			NumLeechs: 1,
			TotalSize: 16 << 30,
		}
		peersByTorrent[index] = make(map[string]qB_PeerStruct, screenshotPeerCount/torrentCount+1)
	}
	for index, peerIP := range peerIPs {
		peerInfo := peers[peerIP]
		peerPort := 0
		for port := range peerInfo.Port {
			peerPort = port
		}
		torrentIndex := index % torrentCount
		peersByTorrent[torrentIndex][peerIP+":"+strconv.Itoa(peerPort)] = qB_PeerStruct{
			IP:         peerIP,
			Port:       peerPort,
			Client:     peerInfo.Client,
			PeerID:     peerInfo.ID,
			Progress:   0.5,
			Downloaded: peerInfo.Downloaded,
			Uploaded:   peerInfo.Uploaded,
			DlSpeed:    1 << 20,
			UpSpeed:    1 << 19,
		}
	}
	torrentPayload, err := json.Marshal(torrents)
	if err != nil {
		tb.Fatal(err)
	}
	peerPayloads := make(map[string][]byte, torrentCount)
	for index, torrent := range torrents {
		payload, err := json.Marshal(qB_TorrentPeersStruct{FullUpdate: true, Peers: peersByTorrent[index]})
		if err != nil {
			tb.Fatal(err)
		}
		peerPayloads[torrent.InfoHash] = payload
	}

	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/torrents/info":
			_, _ = w.Write(torrentPayload)
		case "/v2/sync/torrentPeers":
			payload, exists := peerPayloads[r.URL.Query().Get("hash")]
			if !exists {
				http.Error(w, "unknown torrent", http.StatusNotFound)
				return
			}
			_, _ = w.Write(payload)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))
	tb.Cleanup(server.Close)
	return server, &requestCount
}

func InstallScreenshotScaleQBClient(tb testing.TB, server *httptest.Server) {
	tb.Helper()
	oldHTTPClient := httpClient
	oldCurrentClient := currentClient
	httpClient = *server.Client()
	currentClient = &QBClient{}
	config.ClientURL = server.URL
	config.IgnoreNoLeechersTorrent = false
	config.IgnorePTTorrent = false
	config.SleepTime = 0
	config.UseShadowBan = false
	config.SyncServerURL = ""
	config.BTNConfigureURL = ""
	tb.Cleanup(func() {
		httpClient = oldHTTPClient
		currentClient = oldCurrentClient
	})
}

func NewQBFormBenchmarkServer(tb testing.TB) *httptest.Server {
	tb.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.PostForm.Get("peers") == "" {
			http.Error(w, "invalid peers form", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("Ok."))
	}))
	tb.Cleanup(server.Close)
	return server
}

func TestScreenshotScalePeerOperations(t *testing.T) {
	peers, peerIPs := MakeScreenshotScalePeers()
	InstallScreenshotScalePeers(t, peers)

	totalIPs, totalPorts := GetWebUIBlockStats()
	if totalIPs != screenshotPeerCount || totalPorts != screenshotPeerCount {
		t.Fatalf("stats=%d IPs/%d ports, want %d/%d", totalIPs, totalPorts, screenshotPeerCount, screenshotPeerCount)
	}
	if snapshot := GetWebUIBlockPeers(); len(snapshot) != screenshotPeerCount {
		t.Fatalf("snapshot peer count=%d, want %d", len(snapshot), screenshotPeerCount)
	}
	initial := GetWebUIBlockPeerSync("")
	if !initial.Reset || len(initial.Peers) != screenshotPeerCount {
		t.Fatalf("initial sync reset=%t peers=%d", initial.Reset, len(initial.Peers))
	}
	emptyDelta := GetWebUIBlockPeerSync("0")
	if emptyDelta.Reset || len(emptyDelta.Peers) != 0 || len(emptyDelta.RemovedIP) != 0 {
		t.Fatalf("empty delta=%#v", emptyDelta)
	}
	for index := 0; index < 1000; index++ {
		peerIP := peerIPs[index*len(peerIPs)/1000]
		peer := peers[peerIP]
		for port := range peer.Port {
			if !IsBlockedPeer(peerIP, port, false) {
				t.Fatalf("random peer lookup missed %s:%d", peerIP, port)
			}
		}
	}
	if cleaned := ClearBlockPeer(); cleaned != 0 {
		t.Fatalf("cleaned=%d non-expired peers", cleaned)
	}
}

func TestScreenshotScaleQBSteadyStateCycle(t *testing.T) {
	peers, peerIPs := MakeScreenshotScalePeers()
	InstallScreenshotScalePeers(t, peers)
	server, requestCount := MakeScreenshotScaleQBServer(t, peers, peerIPs, 100)
	InstallScreenshotScaleQBClient(t, server)
	config.WebUI = true

	var taskWait sync.WaitGroup
	taskWait.Add(1)
	go func() {
		defer taskWait.Done()
		Task()
	}()
	for index := 0; index < 3; index++ {
		WebUI_GetStatus(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/api/status", nil))
		WebUI_GetPeers(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/api/peers?sync=1&cursor=0", nil))
		WebUI_GetLogs(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/api/logs", nil))
	}
	taskWait.Wait()

	if got := atomic.LoadInt64(requestCount); got != 101 {
		t.Fatalf("qB request count=%d, want 101", got)
	}
	if totalIPs, totalPorts := GetWebUIBlockStats(); totalIPs != screenshotPeerCount || totalPorts != screenshotPeerCount {
		t.Fatalf("post-cycle stats=%d IPs/%d ports", totalIPs, totalPorts)
	}
	webUIPeerSyncMutex.Lock()
	cursor := webUIPeerSyncCursor
	eventCount := len(webUIPeerSyncEvents)
	webUIPeerSyncMutex.Unlock()
	if cursor != 0 || eventCount != 0 {
		t.Fatalf("steady-state updates produced cursor=%d events=%d", cursor, eventCount)
	}
}

func BenchmarkScreenshotScaleOperations(b *testing.B) {
	peers, peerIPs := MakeScreenshotScalePeers()
	InstallScreenshotScalePeers(b, peers)
	targetIP := peerIPs[len(peerIPs)/2]
	targetPeer := peers[targetIP]
	targetPort := 0
	for port := range targetPeer.Port {
		targetPort = port
	}

	b.Run("BlockedPeerLookup", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			if !IsBlockedPeer(targetIP, targetPort, false) {
				b.Fatal("peer lookup missed")
			}
		}
	})

	b.Run("ExistingPeerUpdate", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			AddBlockPeer("CheckPeer", "Bad-Port", targetIP, targetPort, "benchmark", "peer", "client", int64(index), int64(index))
		}
	})

	b.Run("CleanupScanNoExpiredPeers", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			if cleaned := ClearBlockPeer(); cleaned != 0 {
				b.Fatalf("cleaned=%d", cleaned)
			}
		}
	})

	b.Run("WebUIStats", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			GetWebUIBlockStats()
		}
	})

	b.Run("WebUIFullSnapshot", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			if snapshot := GetWebUIBlockPeers(); len(snapshot) != screenshotPeerCount {
				b.Fatalf("snapshot peers=%d", len(snapshot))
			}
		}
	})

	b.Run("WebUIFullSyncJSON", func(b *testing.B) {
		request := httptest.NewRequest(http.MethodGet, "http://example.com/api/peers?sync=1", nil)
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			recorder := httptest.NewRecorder()
			WebUI_GetPeers(recorder, request)
			if recorder.Code != http.StatusOK || recorder.Body.Len() == 0 {
				b.Fatalf("full sync status=%d bytes=%d", recorder.Code, recorder.Body.Len())
			}
		}
	})

	b.Run("WebUIEmptyDelta", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			response := GetWebUIBlockPeerSync("0")
			if response.Reset || len(response.Peers) != 0 {
				b.Fatal("unexpected non-empty delta")
			}
		}
	})

	b.Run("WebUIEmptyDeltaJSON", func(b *testing.B) {
		request := httptest.NewRequest(http.MethodGet, "http://example.com/api/peers?sync=1&cursor=0", nil)
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			recorder := httptest.NewRecorder()
			WebUI_GetPeers(recorder, request)
			if recorder.Code != http.StatusOK || recorder.Body.Len() == 0 {
				b.Fatalf("empty delta status=%d bytes=%d", recorder.Code, recorder.Body.Len())
			}
		}
	})

	b.Run("ForcedRuntimeGC", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			runtime.GC()
		}
	})

	b.Run("QBittorrentSubmit", func(b *testing.B) {
		oldHTTPClient := httpClient
		server := NewQBFormBenchmarkServer(b)
		httpClient = *server.Client()
		config.ClientURL = server.URL
		b.Cleanup(func() {
			httpClient = oldHTTPClient
		})
		b.ReportAllocs()
		b.ReportMetric(screenshotPeerCount, "peers/op")
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			if !QB_SubmitBlockPeer(peers) {
				b.Fatal("qBittorrent submission failed")
			}
		}
	})

	b.Run("QBittorrentBanAllPortSingleIPv4", func(b *testing.B) {
		oldHTTPClient := httpClient
		server := NewQBFormBenchmarkServer(b)
		httpClient = *server.Client()
		config.ClientURL = server.URL
		config.BanAllPort = true
		b.Cleanup(func() {
			httpClient = oldHTTPClient
			config.BanAllPort = false
		})
		allPortPeer := map[string]BlockPeerInfoStruct{
			"192.0.2.20": {Port: map[int]bool{6881: true}},
		}
		b.ReportAllocs()
		b.ReportMetric(2*65536, "endpoints/op")
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			if !QB_SubmitBlockPeer(allPortPeer) {
				b.Fatal("qBittorrent all-port submission failed")
			}
		}
	})
}

func BenchmarkScreenshotScaleQBSteadyStateRuntimeCycle(b *testing.B) {
	peers, peerIPs := MakeScreenshotScalePeers()
	InstallScreenshotScalePeers(b, peers)
	server, requestCount := MakeScreenshotScaleQBServer(b, peers, peerIPs, 100)
	InstallScreenshotScaleQBClient(b, server)
	config.WebUI = true

	logBufferMutex.Lock()
	oldLogBuffer := logBuffer
	logBuffer = make([]string, logBufferMaxSize)
	for index := range logBuffer {
		logBuffer[index] = fmt.Sprintf("[%d][Task] synthetic benchmark log entry", index)
	}
	logBufferMutex.Unlock()
	b.Cleanup(func() {
		logBufferMutex.Lock()
		logBuffer = oldLogBuffer
		logBufferMutex.Unlock()
	})

	statusRequest := httptest.NewRequest(http.MethodGet, "http://example.com/api/status", nil)
	peerRequest := httptest.NewRequest(http.MethodGet, "http://example.com/api/peers?sync=1&cursor=0", nil)
	logRequest := httptest.NewRequest(http.MethodGet, "http://example.com/api/logs", nil)
	pollWebUI := func() {
		for poll := 0; poll < 3; poll++ {
			WebUI_GetStatus(httptest.NewRecorder(), statusRequest)
			WebUI_GetPeers(httptest.NewRecorder(), peerRequest)
			WebUI_GetLogs(httptest.NewRecorder(), logRequest)
		}
	}
	runCycles := func(b *testing.B, concurrentWebUI bool) {
		b.Helper()
		b.ReportAllocs()
		b.ReportMetric(101, "qB-http-requests/op")
		b.ReportMetric(screenshotPeerCount, "peers/op")
		b.ReportMetric(6, "simulated-sec/op")
		b.ResetTimer()
		for cycle := 0; cycle < b.N; cycle++ {
			beforeRequests := atomic.LoadInt64(requestCount)
			if concurrentWebUI {
				var webUIWait sync.WaitGroup
				webUIWait.Add(1)
				go func() {
					defer webUIWait.Done()
					pollWebUI()
				}()
				Task()
				webUIWait.Wait()
			} else {
				Task()
				pollWebUI()
			}
			if cycle%10 == 9 {
				runtime.GC()
			}
			if requests := atomic.LoadInt64(requestCount) - beforeRequests; requests != 101 {
				b.Fatalf("qB requests=%d, want 101", requests)
			}
			currentTimestamp += int64(config.Interval)
		}
	}
	b.Run("SequentialWebUI", func(b *testing.B) { runCycles(b, false) })
	b.Run("ConcurrentWebUI", func(b *testing.B) { runCycles(b, true) })
}

func BenchmarkQBPeerSynchronization(b *testing.B) {
	peers, ips := MakeScreenshotScalePeers()
	InstallScreenshotScalePeers(b, peers)
	values := make(map[string]qB_PeerStruct, len(ips))
	for _, ip := range ips {
		values[ip] = qB_PeerStruct{IP: ip, Port: 6881, Client: "client", Uploaded: 100}
	}
	full, err := json.Marshal(qB_TorrentPeersStruct{RID: 1, FullUpdate: true, Peers: values})
	if err != nil {
		b.Fatal(err)
	}
	var responseBytes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := []byte(`{"rid":2,"peers":{}}`)
		if r.URL.Query().Get("rid") == "0" {
			payload = full
		}
		responseBytes.Add(int64(len(payload)))
		w.Write(payload)
	}))
	b.Cleanup(server.Close)
	InstallScreenshotScaleQBClient(b, server)
	for _, mode := range []string{"Full", "EmptyDelta"} {
		b.Run(mode, func(b *testing.B) {
			client := &QBClient{}
			if peers, _ := client.FetchTorrentPeers(&Torrent{Hash: "hash"}); len(peers) != screenshotPeerCount {
				b.Fatal("initial snapshot failed")
			}
			startBytes := responseBytes.Load()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if mode == "Full" {
					client.peerRID = 0
				}
				if peers, _ := client.FetchTorrentPeers(&Torrent{Hash: "hash"}); len(peers) != screenshotPeerCount {
					b.Fatal("snapshot lost peers")
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(responseBytes.Load()-startBytes)/float64(b.N), "response-B/op")
		})
	}
}
