package main

import (
	"net"
	"strings"
	"testing"
)

func TestClearBlockPeerExecutesUnbanCommand(t *testing.T) {
	oldConfig := *config
	oldBlockPeerMap := blockPeerMap
	oldBlockCIDRMap := blockCIDRMap
	oldCurrentTimestamp := currentTimestamp
	oldLastCleanTimestamp := lastCleanTimestamp
	oldExecPeerCommand := execPeerCommand
	defer func() {
		tmpConf := oldConfig
	config = &tmpConf
		blockPeerMap = oldBlockPeerMap
		blockCIDRMap = oldBlockCIDRMap
		currentTimestamp = oldCurrentTimestamp
		lastCleanTimestamp = oldLastCleanTimestamp
		execPeerCommand = oldExecPeerCommand
	}()

	tmpConf := oldConfig
	config = &tmpConf
	config.CleanInterval = 0
	config.BanTime = 1
	config.ExecCommand_Unban = "unban {peerIP} {peerPort} {torrentInfoHash}"

	currentTimestamp = 10
	lastCleanTimestamp = 0
	blockPeerMap = map[string]BlockPeerInfoStruct{
		"1.2.3.4": {
			Timestamp: 1,
			Port:      map[int]bool{6881: true, 6882: true},
			InfoHash:  "hash-a",
		},
	}
	blockCIDRMap = map[string]BlockCIDRInfoStruct{}

	var commands []string
	execPeerCommand = func(command string) (bool, string, string) {
		commands = append(commands, command)
		return true, "", ""
	}

	cleanCount := ClearBlockPeer()
	if cleanCount != 1 {
		t.Fatalf("cleanCount=%d, want 1", cleanCount)
	}
	if len(commands) != 2 {
		t.Fatalf("len(commands)=%d, want 2", len(commands))
	}
	for _, command := range commands {
		if !strings.Contains(command, "1.2.3.4") || !strings.Contains(command, "hash-a") {
			t.Fatalf("unexpected command: %q", command)
		}
	}
}

func TestClearBlockPeerSchedulesNextScanWhenNothingExpires(t *testing.T) {
	oldConfig := *config
	oldBlockPeerMap := blockPeerMap
	oldBlockCIDRMap := blockCIDRMap
	oldCurrentTimestamp := currentTimestamp
	oldLastCleanTimestamp := lastCleanTimestamp
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		blockPeerMap = oldBlockPeerMap
		blockCIDRMap = oldBlockCIDRMap
		currentTimestamp = oldCurrentTimestamp
		lastCleanTimestamp = oldLastCleanTimestamp
	})

	testConfig := oldConfig
	testConfig.CleanInterval = 10
	testConfig.BanTime = 1000
	testConfig.ExecCommand_Unban = ""
	testConfig.WebUI = false
	config = &testConfig
	blockPeerMap = map[string]BlockPeerInfoStruct{
		"1.2.3.4": {Timestamp: 99, Port: map[int]bool{6881: true}},
	}
	blockCIDRMap = map[string]BlockCIDRInfoStruct{}
	lastCleanTimestamp = 0
	currentTimestamp = 100

	if count := ClearBlockPeer(); count != 0 {
		t.Fatalf("first clean count=%d, want 0", count)
	}
	if lastCleanTimestamp != 100 {
		t.Fatalf("lastCleanTimestamp=%d, want 100", lastCleanTimestamp)
	}

	peer := blockPeerMap["1.2.3.4"]
	peer.Timestamp = 0
	blockPeerMap["1.2.3.4"] = peer
	testConfig.BanTime = 1
	currentTimestamp = 105
	if count := ClearBlockPeer(); count != 0 {
		t.Fatalf("early clean count=%d, want 0", count)
	}
	if _, exists := blockPeerMap["1.2.3.4"]; !exists {
		t.Fatal("peer was scanned before the clean interval elapsed")
	}

	currentTimestamp = 111
	if count := ClearBlockPeer(); count != 1 {
		t.Fatalf("scheduled clean count=%d, want 1", count)
	}
}

func TestAddBlockCIDRStoresFirstPeerIP(t *testing.T) {
	oldBlockCIDRMap := blockCIDRMap
	oldCurrentTimestamp := currentTimestamp
	defer func() {
		blockCIDRMap = oldBlockCIDRMap
		currentTimestamp = oldCurrentTimestamp
	}()

	blockCIDRMap = map[string]BlockCIDRInfoStruct{}
	currentTimestamp = 100

	_, peerNet, err := net.ParseCIDR("1.2.3.4/24")
	if err != nil {
		t.Fatalf("ParseCIDR failed: %v", err)
	}

	AddBlockCIDR("1.2.3.4", peerNet)

	info, exists := blockCIDRMap[peerNet.String()]
	if !exists {
		t.Fatalf("expected CIDR %q to be stored", peerNet.String())
	}
	if !info.IPs["1.2.3.4"] {
		t.Fatalf("expected first peer IP to be recorded in CIDR membership: %#v", info.IPs)
	}
}
