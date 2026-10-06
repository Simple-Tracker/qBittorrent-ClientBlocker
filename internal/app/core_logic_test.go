package app

import (
	"net"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/stats"
	"github.com/dlclark/regexp2"
)

func TestUploadProgressRules(t *testing.T) {
	oldConfig := *config
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
	})
	testConfig := oldConfig
	testConfig.BanByProgressUploaded = true
	testConfig.BanByPUStartMB = 1
	testConfig.BanByPUStartPercent = 2
	testConfig.BanByPUAntiErrorRatio = 3
	testConfig.BanByRelativePUStartMB = 1
	testConfig.BanByRelativePUStartPercent = 3
	testConfig.BanByRelativePUAntiErrorRatio = 3
	config = &testConfig

	const torrentSize = int64(100 << 20)
	if !statistics.IsProgressNotMatchUploaded(torrentSize, 0.01, 10<<20) {
		t.Fatal("absolute upload/progress mismatch was not detected")
	}
	if statistics.IsProgressNotMatchUploaded(torrentSize, 0.5, 10<<20) {
		t.Fatal("valid absolute upload/progress ratio was rejected")
	}

	previous := stats.PeerInfoStruct{Progress: 0.10, Uploaded: 1 << 20}
	current := stats.PeerInfoStruct{Progress: 0.11, Uploaded: 10 << 20}
	if uploaded := statistics.IsProgressNotMatchUploaded_Relative(torrentSize, current, previous); uploaded != 9<<20 {
		t.Fatalf("relative mismatch upload=%d, want %d", uploaded, 9<<20)
	}
	current.Progress = 0.9
	if uploaded := statistics.IsProgressNotMatchUploaded_Relative(torrentSize, current, previous); uploaded != 0 {
		t.Fatalf("valid relative progress returned upload=%d", uploaded)
	}
}

func TestIPAggregationAndUploadDelta(t *testing.T) {
	oldConfig := *config
	oldIPMap := statistics.State().IPMap
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		statistics.State().IPMap = oldIPMap
	})
	testConfig := oldConfig
	testConfig.MaxIPPortCount = 2
	testConfig.IPUpCheckIncrementMB = 2
	config = &testConfig
	statistics.State().IPMap = map[string]stats.IPInfoStruct{}

	statistics.AddIPInfo(nil, "203.0.113.10", 6881, "torrent-a", 0, 2<<20)
	statistics.AddIPInfo(nil, "203.0.113.10", 6882, "torrent-b", 0, 7<<20)
	baseline := make(map[string]stats.IPInfoStruct)
	stats.DeepCopyIPMap(statistics.State().IPMap, baseline)
	statistics.AddIPInfo(nil, "203.0.113.10", 6881, "torrent-a", 4<<20, 5<<20)
	statistics.AddIPInfo(nil, "203.0.113.10", 6882, "torrent-b", 6<<20, 8<<20)
	info := statistics.State().IPMap["203.0.113.10"]
	if len(info.Port) != 2 || len(info.TorrentDownloaded) != 2 || len(info.TorrentUploaded) != 2 {
		t.Fatalf("unexpected aggregated IP info: %#v", info)
	}

	previous := baseline["203.0.113.10"]
	if deltaMB := statistics.IsIPTooHighUploaded(info, previous); deltaMB != 4 {
		t.Fatalf("upload delta=%d MB, want 4", deltaMB)
	}
}

func TestTorrentAggregation(t *testing.T) {
	oldConfig := *config
	oldTorrentMap := statistics.State().TorrentMap
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		statistics.State().TorrentMap = oldTorrentMap
	})
	testConfig := oldConfig
	testConfig.BanByRelativeProgressUploaded = true
	config = &testConfig
	statistics.State().TorrentMap = map[string]stats.TorrentInfoStruct{}

	_, peerNet, err := net.ParseCIDR("203.0.113.10/32")
	if err != nil {
		t.Fatal(err)
	}
	statistics.AddTorrentInfo("torrent-a", 100<<20, peerNet, "203.0.113.10", 6881, 0.1, 10, 20, "peer-a", "client-a")
	statistics.AddTorrentInfo("torrent-a", 100<<20, peerNet, "203.0.113.10", 6882, 0.2, 30, 40, "peer-a", "client-a")
	peer := statistics.State().TorrentMap["torrent-a"].Peers["203.0.113.10"]
	if len(peer.Port) != 2 || peer.Progress != 0.2 || peer.Uploaded != 60 {
		t.Fatalf("unexpected aggregated torrent peer: %#v", peer)
	}
}

func TestCheckPeerCoreDecisions(t *testing.T) {
	oldConfig := *config
	oldBlockPeerMap := blockPeerMap
	oldBlockCIDRMap := blockCIDRMap
	oldCurrentTimestamp := currentTimestamp
	oldClientType := currentClientType
	oldQBMethod := qB_useNewBanPeersMethod
	oldBTNConfig := btnConfig
	oldSyncRules := syncServer_CompiledRules
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		blockPeerMap = oldBlockPeerMap
		blockCIDRMap = oldBlockCIDRMap
		currentTimestamp = oldCurrentTimestamp
		currentClientType = oldClientType
		qB_useNewBanPeersMethod = oldQBMethod
		btnConfig = oldBTNConfig
		syncServer_CompiledRules = oldSyncRules
		EraseSyncMap(&blockListCompiled)
		EraseSyncMap(&ipBlockListCompiled)
	})

	testConfig := oldConfig
	testConfig.BanIPCIDR = "/24"
	testConfig.BanIP6CIDR = "/128"
	testConfig.ExecCommand_Ban = ""
	testConfig.IgnoreEmptyPeer = false
	testConfig.PortBlockList = nil
	testConfig.WebUI = false
	config = &testConfig
	blockPeerMap = map[string]BlockPeerInfoStruct{
		"203.0.113.1": {Timestamp: 1, Port: map[int]bool{6881: true}},
	}
	blockCIDRMap = map[string]BlockCIDRInfoStruct{}
	currentTimestamp = 100
	currentClientType = "qBittorrent"
	qB_useNewBanPeersMethod = true
	btnConfig = nil
	syncServer_CompiledRules = nil
	EraseSyncMap(&blockListCompiled)
	EraseSyncMap(&ipBlockListCompiled)

	if status, _ := CheckPeer("203.0.113.1", 6881, "peer", "client", 1, 1, 0.5, 1, 1, "torrent", 100); status != 2 {
		t.Fatalf("blocked port status=%d, want 2", status)
	}
	testConfig.PortBlockList = []uint32{6882}
	if status, _ := CheckPeer("203.0.113.2", 6882, "peer", "client", 1, 1, 0.5, 1, 1, "torrent", 100); status != 1 {
		t.Fatalf("blocked configured port status=%d, want 1", status)
	}
	testConfig.PortBlockList = nil
	if status, peerNet := CheckPeer("203.0.114.3", 6883, "peer", "client", 1, 1, 0.5, 1, 1, "torrent", 100); status != 0 || peerNet == nil {
		t.Fatalf("valid peer status=%d net=%v, want 0/non-nil", status, peerNet)
	}
	if status, _ := CheckPeer("127.0.0.1", 6881, "peer", "client", 1, 1, 0.5, 1, 1, "torrent", 100); status != -1 {
		t.Fatalf("private peer status=%d, want -1", status)
	}

	blockListCompiled.Store("test-client", regexp2.MustCompile("BlockedClient", 0))
	if status, _ := CheckPeer("203.0.116.1", 6881, "peer", "BlockedClient", 1, 1, 0.5, 1, 1, "torrent", 100); status != 1 {
		t.Fatalf("client rule status=%d, want 1", status)
	}
	EraseSyncMap(&blockListCompiled)

	ipBlockListCompiled.Store("203.0.117.1", ParseIPCIDR("203.0.117.1"))
	if status, _ := CheckPeer("203.0.117.1", 6881, "peer", "client", 1, 1, 0.5, 1, 1, "torrent", 100); status != 3 {
		t.Fatalf("IP rule status=%d, want 3", status)
	}
	EraseSyncMap(&ipBlockListCompiled)

	AddBlockCIDR("203.0.119.1", ParseIPCIDR("203.0.119.0/24"))
	if status, _ := CheckPeer("203.0.119.2", 6881, "peer", "client", 1, 1, 0.5, 1, 1, "torrent", 100); status != 1 {
		t.Fatalf("CIDR rule status=%d, want 1", status)
	}
	if status, _ := CheckPeer("203.0.120.1", 6881, "peer", "client", 0, 0, 0.5, 1, 1, "torrent", 100); status != -2 {
		t.Fatalf("idle peer status=%d, want -2", status)
	}
	testConfig.IgnoreEmptyPeer = true
	if status, _ := CheckPeer("203.0.121.1", 6881, "", "", 1, 1, 0.5, 1, 1, "torrent", 100); status != -2 {
		t.Fatalf("empty peer status=%d, want -2", status)
	}
	testConfig.IgnoreEmptyPeer = false
	testConfig.BanByProgressUploaded = true
	testConfig.BanByPUStartMB = 1
	testConfig.BanByPUStartPercent = 2
	testConfig.BanByPUAntiErrorRatio = 3
	if status, _ := CheckPeer("203.0.122.1", 6881, "peer", "client", 1, 1, 0.01, 1, 10<<20, "torrent", 100<<20); status != 1 {
		t.Fatalf("progress/upload rule status=%d, want 1", status)
	}
}
