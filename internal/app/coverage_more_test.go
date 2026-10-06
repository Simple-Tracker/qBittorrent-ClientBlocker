package app

import (
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/stats"
	"github.com/dlclark/regexp2"
)

func TestPrepareEnvParsesConfigFlagsWithoutChangingDirectory(t *testing.T) {
	originalArgs := os.Args
	originalFlags := flag.CommandLine
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	originalConfig := ConfigSnapshot()
	originalConfigFilename := configFilename
	originalAdditionalFilename := additionConfigFilename
	originalShortVersion := shortFlag_ShowVersion
	originalLongVersion := longFlag_ShowVersion
	originalStartDelay := startDelay
	originalNoChdir := noChdir
	originalNeedRegHotKey := needRegHotKey
	originalNeedHideWindow := needHideWindow
	originalNeedHideSystray := needHideSystray
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlags
		_ = os.Chdir(originalWorkingDirectory)
		ReplaceConfig(originalConfig)
		configFilename = originalConfigFilename
		additionConfigFilename = originalAdditionalFilename
		shortFlag_ShowVersion = originalShortVersion
		longFlag_ShowVersion = originalLongVersion
		startDelay = originalStartDelay
		noChdir = originalNoChdir
		needRegHotKey = originalNeedRegHotKey
		needHideWindow = originalNeedHideWindow
		needHideSystray = originalNeedHideSystray
	})

	primary := filepath.Join(t.TempDir(), "custom.json")
	additional := filepath.Join(t.TempDir(), "additional.json")
	flag.CommandLine = flag.NewFlagSet("prepare-env", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = []string{"qbcb-test", "-c", primary, "-config_additional", additional, "-debug", "-startdelay", "3", "-nochdir", "-reghotkey=false", "-hidewindow", "-hidesystray"}

	if !PrepareEnv() {
		t.Fatal("PrepareEnv unexpectedly requested exit")
	}
	if configFilename != primary || additionConfigFilename != additional {
		t.Fatalf("config paths primary=%q additional=%q", configFilename, additionConfigFilename)
	}
	if !ConfigSnapshot().Debug || startDelay != 3 || !noChdir || needRegHotKey || !needHideWindow || !needHideSystray {
		t.Fatalf("flags were not applied: config=%#v startDelay=%d noChdir=%v regHotKey=%v hideWindow=%v hideSystray=%v", ConfigSnapshot(), startDelay, noChdir, needRegHotKey, needHideWindow, needHideSystray)
	}

	longPrimary := filepath.Join(t.TempDir(), "long.json")
	shortAdditional := filepath.Join(t.TempDir(), "short-additional.json")
	shortFlag_ShowVersion = false
	longFlag_ShowVersion = false
	noChdir = false
	flag.CommandLine = flag.NewFlagSet("prepare-env-alternate", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = []string{"qbcb-test", "-config", longPrimary, "-ca", shortAdditional}
	if !PrepareEnv() {
		t.Fatal("alternate PrepareEnv unexpectedly requested exit")
	}
	if configFilename != longPrimary || additionConfigFilename != shortAdditional {
		t.Fatalf("alternate config paths primary=%q additional=%q", configFilename, additionConfigFilename)
	}
}

func TestPrepareEnvVersionFlagRequestsExit(t *testing.T) {
	originalArgs := os.Args
	originalFlags := flag.CommandLine
	originalShortVersion := shortFlag_ShowVersion
	originalLongVersion := longFlag_ShowVersion
	t.Cleanup(func() {
		os.Args = originalArgs
		flag.CommandLine = originalFlags
		shortFlag_ShowVersion = originalShortVersion
		longFlag_ShowVersion = originalLongVersion
	})

	shortFlag_ShowVersion = false
	longFlag_ShowVersion = false
	flag.CommandLine = flag.NewFlagSet("prepare-env-version", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = []string{"qbcb-test", "-version"}
	if PrepareEnv() {
		t.Fatal("PrepareEnv should stop after showing the version")
	}
}

func TestLoadConfigReadAndParseErrors(t *testing.T) {
	directory := t.TempDir()
	target := ConfigStruct{}
	if status := LoadConfig(filepath.Join(directory, "missing.json"), true, &target); status != -5 {
		t.Fatalf("missing required config status=%d", status)
	}

	unreadable := filepath.Join(directory, "directory.json")
	if err := os.Mkdir(unreadable, 0o700); err != nil {
		t.Fatal(err)
	}
	if status := LoadConfig(unreadable, true, &target); status != -3 {
		t.Fatalf("directory config status=%d, want -3", status)
	}

	malformedTOML := filepath.Join(directory, "malformed.toml")
	if err := os.WriteFile(malformedTOML, []byte("Interval = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if status := LoadConfig(malformedTOML, true, &target); status != -4 {
		t.Fatalf("malformed TOML status=%d, want -4", status)
	}

	unknownExtension := filepath.Join(directory, "config.txt")
	if err := os.WriteFile(unknownExtension, []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if status := LoadConfig(unknownExtension, true, &target); status != 0 {
		t.Fatalf("unknown extension status=%d, want 0", status)
	}

	t.Cleanup(func() {
		lastModMutex.Lock()
		delete(configLastMod, malformedTOML)
		delete(configLastMod, unknownExtension)
		lastModMutex.Unlock()
	})
}

func TestLocalRuleFileErrorAndJSONBranches(t *testing.T) {
	originalConfig := ConfigSnapshot()
	t.Cleanup(func() {
		ReplaceConfig(originalConfig)
		EraseSyncMap(&ruleStore.BlockList)
		EraseSyncMap(&ruleStore.IPBlockList)
	})

	testConfig := *originalConfig
	testConfig.BlockListFile = nil
	testConfig.IPBlockListFile = nil
	ReplaceConfig(&testConfig)
	if !SetBlockListFromFile() || !SetIPBlockListFromFile() {
		t.Fatal("empty local rule lists should succeed")
	}

	directory := t.TempDir()
	missing := filepath.Join(directory, "missing.txt")
	testConfig.BlockListFile = []string{missing}
	testConfig.IPBlockListFile = []string{missing}
	ReplaceConfig(&testConfig)
	if SetBlockListFromFile() || SetIPBlockListFromFile() {
		t.Fatal("missing local rule files should fail")
	}

	blockJSON := filepath.Join(directory, "block.json")
	ipJSON := filepath.Join(directory, "ip.json")
	if err := os.WriteFile(blockJSON, []byte(`["Client.*", "["]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ipJSON, []byte(`["192.0.2.1", "invalid"]`), 0o600); err != nil {
		t.Fatal(err)
	}
	testConfig.BlockListFile = []string{blockJSON}
	testConfig.IPBlockListFile = []string{ipJSON}
	ReplaceConfig(&testConfig)
	if !SetBlockListFromFile() || !SetIPBlockListFromFile() {
		t.Fatal("JSON rule files should load")
	}
	if _, ok := ruleStore.BlockList.Load("Client.*"); !ok {
		t.Fatal("valid block-list expression was not compiled")
	}
	if _, ok := ruleStore.IPBlockList.Load("192.0.2.1"); !ok {
		t.Fatal("valid IP rule was not compiled")
	}

	badBlockJSON := filepath.Join(directory, "bad-block.json")
	badIPJSON := filepath.Join(directory, "bad-ip.json")
	if err := os.WriteFile(badBlockJSON, []byte(`[`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badIPJSON, []byte(`[`), 0o600); err != nil {
		t.Fatal(err)
	}
	testConfig.BlockListFile = []string{badBlockJSON}
	testConfig.IPBlockListFile = []string{badIPJSON}
	ReplaceConfig(&testConfig)
	if !SetBlockListFromFile() || !SetIPBlockListFromFile() {
		t.Fatal("parse errors should not abort local rule refresh")
	}

	readFailure := filepath.Join(directory, "rules.txt")
	if err := os.Mkdir(readFailure, 0o700); err != nil {
		t.Fatal(err)
	}
	testConfig.BlockListFile = []string{readFailure}
	testConfig.IPBlockListFile = []string{readFailure}
	ReplaceConfig(&testConfig)
	if SetBlockListFromFile() || SetIPBlockListFromFile() {
		t.Fatal("directories should fail local rule reads")
	}

	largeBlockFile := filepath.Join(directory, "large-block.txt")
	largeFile, err := os.Create(largeBlockFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := largeFile.Truncate(8388609); err != nil {
		_ = largeFile.Close()
		t.Fatal(err)
	}
	if err := largeFile.Close(); err != nil {
		t.Fatal(err)
	}
	testConfig.BlockListFile = []string{largeBlockFile}
	ReplaceConfig(&testConfig)
	if !SetBlockListFromFile() {
		t.Fatal("oversized block-list file should be skipped without aborting refresh")
	}
}

func TestUtilityAndCrashErrorBranches(t *testing.T) {
	originalConfig := ConfigSnapshot()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ReplaceConfig(originalConfig)
		_ = os.Chdir(originalWorkingDirectory)
	})

	testConfig := *originalConfig
	testConfig.BanIPCIDR = "/24"
	testConfig.BanIP6CIDR = "/64"
	ReplaceConfig(&testConfig)
	if cidr := ParseIPCIDRByConfig("192.0.2.9"); cidr == nil || cidr.String() != "192.0.2.0/24" {
		t.Fatalf("IPv4 configured CIDR=%v", cidr)
	}
	if cidr := ParseIPCIDRByConfig("2001:db8::9"); cidr == nil || cidr.String() != "2001:db8::/64" {
		t.Fatalf("IPv6 configured CIDR=%v", cidr)
	}
	testConfig.BanIPCIDR = "invalid"
	ReplaceConfig(&testConfig)
	if ParseIPCIDRByConfig("192.0.2.9") != nil {
		t.Fatal("invalid configured CIDR should fail")
	}

	directory := t.TempDir()
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("ipfilter.dat", 0o700); err != nil {
		t.Fatal(err)
	}
	if SaveIPFilter("content") == "" {
		t.Fatal("SaveIPFilter should report a directory write failure")
	}
	if err := os.Remove("ipfilter.dat"); err != nil {
		t.Fatal(err)
	}
	if DeleteIPFilter() {
		t.Fatal("DeleteIPFilter should fail for a missing file")
	}

	logPathFile := filepath.Join(directory, "not-a-directory")
	if err := os.WriteFile(logPathFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	testConfig.LogPath = filepath.Join(logPathFile, "logs")
	ReplaceConfig(&testConfig)
	WriteCrashLog("mkdir-failure", "panic", nil)

	crashDirectory := filepath.Join(directory, "crash-dir")
	if err := os.MkdirAll(filepath.Join(crashDirectory, "crash.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	testConfig.LogPath = crashDirectory
	ReplaceConfig(&testConfig)
	WriteCrashLog("open-failure", "panic", nil)
}

func TestSyncServerFailureAndSchedulingBranches(t *testing.T) {
	originalConfig := ConfigSnapshot()
	originalClient := httpClientExternal
	originalSyncConfig := syncServer_syncConfig
	originalRules := syncServer_CompiledRules
	originalLastSync := atomic.LoadInt64(&syncServer_lastSync)
	originalTimestamp := atomic.LoadInt64(&currentTimestamp)
	originalSubmitting := syncServer_isSubmiting.Load()
	t.Cleanup(func() {
		ReplaceConfig(originalConfig)
		httpClientExternal = originalClient
		syncServer_syncConfig = originalSyncConfig
		syncServer_CompiledRules = originalRules
		atomic.StoreInt64(&syncServer_lastSync, originalLastSync)
		atomic.StoreInt64(&currentTimestamp, originalTimestamp)
		syncServer_isSubmiting.Store(originalSubmitting)
	})

	testConfig := *originalConfig
	testConfig.SyncServerURL = "http://sync.test/config"
	ReplaceConfig(&testConfig)

	httpClientExternal = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network failure")
	})}
	if SyncWithServer_Submit(`{}`) {
		t.Fatal("network failure should fail sync submission")
	}

	for _, body := range []string{
		strings.Repeat("x", 8388609),
		`{`,
		`{"status":"server rejected request"}`,
	} {
		responseBody := body
		httpClientExternal = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return CoverageResponse(http.StatusOK, responseBody, nil), nil
		})}
		if SyncWithServer_Submit(`{}`) {
			t.Fatalf("invalid sync response unexpectedly succeeded (length=%d)", len(body))
		}
	}

	validResponse := `{
		"interval": 1,
		"blockIPRule": {
			"mixed": ["", "# comment", "invalid", "198.51.100.0/24"]
		}
	}`
	httpClientExternal = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return CoverageResponse(http.StatusOK, validResponse, nil), nil
	})}
	if !SyncWithServer_Submit(`{}`) || len(syncServer_CompiledRules) != 1 {
		t.Fatalf("valid mixed sync rules=%#v", syncServer_CompiledRules)
	}
	if matched, reason := SyncServer_CheckPeer(nil); matched || reason != "" {
		t.Fatalf("nil peer match=(%v,%q)", matched, reason)
	}
	syncServer_CompiledRules = append(syncServer_CompiledRules, SyncServer_RuleStruct{Net: nil, Reason: "nil"})
	if matched, _ := SyncServer_CheckPeer(net.ParseIP("203.0.113.1")); matched {
		t.Fatal("nil sync rule unexpectedly matched")
	}

	syncServer_syncConfig = &SyncServer_ConfigStruct{Interval: 0, BlockIPRule: map[string][]string{}}
	atomic.StoreInt64(&syncServer_lastSync, 0)
	atomic.StoreInt64(&currentTimestamp, 100)
	syncServer_isSubmiting.Store(false)
	if !SyncWithServer() {
		t.Fatal("scheduled sync did not start")
	}
	deadline := time.Now().Add(2 * time.Second)
	for syncServer_isSubmiting.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if syncServer_isSubmiting.Load() || syncServer_syncConfig.Interval != 1 {
		t.Fatalf("scheduled sync did not finish: submitting=%v config=%#v", syncServer_isSubmiting.Load(), syncServer_syncConfig)
	}
	if !SyncWithServer() {
		t.Fatal("sync interval guard should succeed")
	}
}

func TestDetectClientSuccessfulBranches(t *testing.T) {
	originalConfig := ConfigSnapshot()
	originalClient := httpClient
	originalExternalClient := httpClientExternal
	originalCurrentClient := currentClient
	originalClientType := currentClientType
	t.Cleanup(func() {
		ReplaceConfig(originalConfig)
		httpClient = originalClient
		httpClientExternal = originalExternalClient
		currentClient = originalCurrentClient
		currentClientType = originalClientType
	})

	httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return CoverageResponse(http.StatusOK, `{}`, nil), nil
	})}
	httpClientExternal = httpClient
	for _, clientType := range []string{"qBittorrent", "Transmission", "BitComet"} {
		testConfig := *originalConfig
		testConfig.ClientType = clientType
		testConfig.ClientURL = "http://client.test"
		ReplaceConfig(&testConfig)
		currentClient = nil
		currentClientType = ""
		if !DetectClient() || currentClient == nil || currentClientType != clientType {
			t.Fatalf("successful detection for %s produced client=%T type=%q", clientType, currentClient, currentClientType)
		}
	}

	testConfig := *originalConfig
	testConfig.ClientURL = "http://client.test"
	ReplaceConfig(&testConfig)
	currentClientType = "qBittorrent"
	httpClient = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return CoverageResponse(http.StatusInternalServerError, "", nil), nil
	})}
	if TestShadowBanAPI() != -1 {
		t.Fatal("failed qBittorrent shadow-ban probe should return -1")
	}
}

func TestTaskGeneratesFilterAfterCleaningExpiredPeer(t *testing.T) {
	originalConfig := ConfigSnapshot()
	originalClient := currentClient
	originalClientType := currentClientType
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	originalBlockPeerMap := blockPeerMap
	originalBlockCIDRMap := blockCIDRMap
	originalIPMap := statistics.State().IPMap
	originalLastIPMap := statistics.State().LastIPMap
	originalTorrentMap := statistics.State().TorrentMap
	originalLastTorrentMap := statistics.State().LastTorrentMap
	originalTimestamp := currentTimestamp
	originalLastClean := lastCleanTimestamp
	originalRules := syncServer_CompiledRules
	t.Cleanup(func() {
		ReplaceConfig(originalConfig)
		currentClient = originalClient
		currentClientType = originalClientType
		blockPeerMap = originalBlockPeerMap
		blockCIDRMap = originalBlockCIDRMap
		statistics.State().IPMap = originalIPMap
		statistics.State().LastIPMap = originalLastIPMap
		statistics.State().TorrentMap = originalTorrentMap
		statistics.State().LastTorrentMap = originalLastTorrentMap
		currentTimestamp = originalTimestamp
		lastCleanTimestamp = originalLastClean
		syncServer_CompiledRules = originalRules
		_ = os.Chdir(originalWorkingDirectory)
		EraseSyncMap(&ruleStore.IPBlockList)
	})

	directory := t.TempDir()
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	testConfig := *originalConfig
	testConfig.ClientURL = "http://client.test"
	testConfig.UseShadowBan = false
	testConfig.CleanInterval = 0
	testConfig.BanTime = 1
	testConfig.GenIPDat = 1
	testConfig.SyncServerURL = ""
	testConfig.IPUploadedCheck = false
	ReplaceConfig(&testConfig)
	stub := &coverageClient{clientType: "test", login: true}
	currentClient = stub
	currentClientType = "test"
	currentTimestamp = 100
	lastCleanTimestamp = 0
	blockPeerMap = map[string]BlockPeerInfoStruct{
		"192.0.2.1": {Timestamp: 1, Port: map[int]bool{6881: true}},
	}
	blockCIDRMap = make(map[string]BlockCIDRInfoStruct)
	statistics.State().IPMap = make(map[string]stats.IPInfoStruct)
	statistics.State().LastIPMap = make(map[string]stats.IPInfoStruct)
	statistics.State().TorrentMap = make(map[string]stats.TorrentInfoStruct)
	statistics.State().LastTorrentMap = make(map[string]stats.TorrentInfoStruct)
	syncServer_CompiledRules = nil
	EraseSyncMap(&ruleStore.IPBlockList)

	Task()
	if len(blockPeerMap) != 0 || stub.normalBan != 1 {
		t.Fatalf("expired peers=%#v ban submissions=%d", blockPeerMap, stub.normalBan)
	}
	if _, err := os.Stat("ipfilter.dat"); err != nil {
		t.Fatalf("generated filter missing: %v", err)
	}

	blockPeerMap["198.51.100.1"] = BlockPeerInfoStruct{Timestamp: 1, Port: map[int]bool{6881: true}}
	currentTimestamp++
	testConfig.IPUploadedCheck = true
	ReplaceConfig(&testConfig)
	if err := os.Remove("ipfilter.dat"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("ipfilter.dat", 0o700); err != nil {
		t.Fatal(err)
	}
	Task()
}

func TestWaitStopAndNonPanicShutdown(t *testing.T) {
	originalReqStopChan := reqStopChan
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reqStopChan = originalReqStopChan
		_ = os.Chdir(originalWorkingDirectory)
	})

	reqStopChan = make(chan struct{})
	close(reqStopChan)
	WaitStop()

	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	Stop(nil, nil)
}

func TestServerConflictAndListenerErrors(t *testing.T) {
	originalConfig := ConfigSnapshot()
	originalClientType := currentClientType
	originalStatus := Server_Status
	originalListeners := Server_Listeners
	t.Cleanup(func() {
		StopServer()
		ReplaceConfig(originalConfig)
		currentClientType = originalClientType
		Server_Status = originalStatus
		Server_Listeners = originalListeners
	})

	if listener, err := CreateListener("not-an-address"); err == nil {
		_ = listener.Close()
		t.Fatal("invalid listener address unexpectedly succeeded")
	}
	if listener, err := CreateListener("[::1]:0"); err == nil {
		_ = listener.Close()
	}

	testConfig := *originalConfig
	testConfig.WebUI = false
	testConfig.Listen = "not-an-address"
	ReplaceConfig(&testConfig)
	currentClientType = "Transmission"
	Server_Status = false
	Server_Listeners = nil
	StartServer()
	if Server_Status {
		t.Fatal("server started with an invalid address")
	}

	Server_Status = true
	StartServer()
	Server_Status = false
	StopServer()
}

func TestPeerCommandAndRareDecisionBranches(t *testing.T) {
	originalConfig := ConfigSnapshot()
	originalBlockPeers := blockPeerMap
	originalBlockCIDRs := blockCIDRMap
	originalRules := syncServer_CompiledRules
	originalClientType := currentClientType
	originalNewBanMethod := qB_useNewBanPeersMethod
	originalTimestamp := currentTimestamp
	t.Cleanup(func() {
		ReplaceConfig(originalConfig)
		blockPeerMap = originalBlockPeers
		blockCIDRMap = originalBlockCIDRs
		syncServer_CompiledRules = originalRules
		currentClientType = originalClientType
		qB_useNewBanPeersMethod = originalNewBanMethod
		currentTimestamp = originalTimestamp
		EraseSyncMap(&ruleStore.IPBlockList)
	})

	testConfig := *originalConfig
	testConfig.BanIPCIDR = "/24"
	testConfig.BanIP6CIDR = "/64"
	testConfig.ExecCommand_Ban = "/usr/bin/true"
	testConfig.IgnoreEmptyPeer = false
	testConfig.IgnoreByDownloaded = 1
	testConfig.Debug = true
	testConfig.Debug_CheckPeer = true
	ReplaceConfig(&testConfig)
	currentTimestamp = 100
	blockPeerMap = map[string]BlockPeerInfoStruct{
		"192.0.2.1": {
			Port:                 map[int]bool{6881: true},
			TorrentDownloaded:    nil,
			TorrentUploaded:      nil,
			TorrentDownloadedRaw: nil,
			TorrentUploadedRaw:   nil,
		},
	}
	blockCIDRMap = make(map[string]BlockCIDRInfoStruct)
	AddBlockPeer("test", "reason", "192.0.2.1", 6882, "hash", "id", "client", 10, 20)
	if peer := blockPeerMap["192.0.2.1"]; peer.Downloaded != 10 || peer.Uploaded != 20 {
		t.Fatalf("reinitialised peer totals=%#v", peer)
	}
	testConfig.ExecCommand_Ban = "/usr/bin/false"
	ReplaceConfig(&testConfig)
	AddBlockPeer("test", "reason", "192.0.2.2", 6881, "", "", "", 1, 2)

	currentClientType = "qBittorrent"
	qB_useNewBanPeersMethod = true
	blockPeerMap["198.51.100.1"] = BlockPeerInfoStruct{Port: map[int]bool{6881: true}}
	if IsBlockedPeer("198.51.100.1", 6882, false) {
		t.Fatal("different qBittorrent port unexpectedly matched")
	}

	peerIDRegex := regexp2.MustCompile(`peer-id`, 0)
	if !MatchBlockList(peerIDRegex, "203.0.113.1", 1, "peer-id", "") {
		t.Fatal("peer ID block-list match failed")
	}
	EraseSyncMap(&ruleStore.IPBlockList)
	ruleStore.IPBlockList.Store("nil", nil)
	ruleStore.IPBlockList.Store("wrong-type", "not-a-network")
	status, _ := CheckPeer("not-an-ip", 6881, "id", "client", 1, 1, 0, 0, 0, "hash", 100)
	if status != 0 {
		t.Fatalf("invalid textual IP status=%d", status)
	}

	syncServer_CompiledRules = []SyncServer_RuleStruct{{Net: ParseIPCIDR("203.0.113.0/24"), Reason: "sync-test"}}
	status, _ = CheckPeer("203.0.113.9", 6881, "id", "client", 1, 1, 0, 0, 0, "hash", 100)
	if status != 3 {
		t.Fatalf("sync-blocked peer status=%d", status)
	}
	blockCount, ipBlockCount, badCount, emptyCount := 0, 0, 0, 0
	ProcessPeer(&Peer{IP: "203.0.113.10", Port: 6881, ID: "id", Client: "client", DlSpeed: 1}, "hash", 100, &blockCount, &ipBlockCount, &badCount, &emptyCount)
	if ipBlockCount != 1 {
		t.Fatalf("sync-blocked ProcessPeer IP count=%d", ipBlockCount)
	}

	syncServer_CompiledRules = nil
	status, _ = CheckPeer("198.51.100.2", 6881, "id", "client", 1, 1, 0, 2*1024*1024, 0, "hash", 100)
	if status != -2 {
		t.Fatalf("download-threshold peer status=%d", status)
	}
}

func TestRemoteRuleSizeAndNotModifiedBranches(t *testing.T) {
	originalConfig := ConfigSnapshot()
	originalExternalClient := httpClientExternal
	originalTimestamp := currentTimestamp
	originalBlockFetch := blockListURLLastFetch
	originalIPFetch := ipBlockListURLLastFetch
	t.Cleanup(func() {
		ReplaceConfig(originalConfig)
		httpClientExternal = originalExternalClient
		currentTimestamp = originalTimestamp
		blockListURLLastFetch = originalBlockFetch
		ipBlockListURLLastFetch = originalIPFetch
	})

	largeBody := strings.Repeat("x", 8388609)
	httpClientExternal = http.Client{Transport: coverageRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.Contains(request.URL.Path, "unchanged") {
			return CoverageResponse(http.StatusNotModified, "", nil), nil
		}
		return CoverageResponse(http.StatusOK, largeBody, nil), nil
	})}
	testConfig := *originalConfig
	testConfig.UpdateInterval = 0
	testConfig.BlockListURL = []string{"http://rules.test/unchanged", "http://rules.test/large"}
	testConfig.IPBlockListURL = []string{"http://rules.test/unchanged", "http://rules.test/large"}
	ReplaceConfig(&testConfig)
	currentTimestamp = 100
	blockListURLLastFetch = 0
	ipBlockListURLLastFetch = 0
	if !SetBlockListFromURL() || !SetIPBlockListFromURL() {
		t.Fatal("remote rule guards should complete successfully")
	}
}
