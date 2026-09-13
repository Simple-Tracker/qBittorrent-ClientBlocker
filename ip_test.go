package main

import "testing"

func TestCheckAllIPColdStart(t *testing.T) {
	oldConfig := ConfigSnapshot()
	oldTimestamp, oldClean := currentTimestamp, lastIPCleanTimestamp
	oldPeers, oldCIDRs := blockPeerMap, blockCIDRMap
	t.Cleanup(func() {
		ReplaceConfig(oldConfig)
		currentTimestamp, lastIPCleanTimestamp = oldTimestamp, oldClean
		blockPeerMap, blockCIDRMap = oldPeers, oldCIDRs
	})
	cfg := *oldConfig
	cfg.MaxIPPortCount = 1
	cfg.IPUploadedCheck = true
	cfg.IPUpCheckIncrementMB = 1
	cfg.IPUpCheckInterval = 1
	cfg.ExecCommand_Ban = ""
	cfg.WebUI = false
	ReplaceConfig(&cfg)
	blockPeerMap = make(map[string]BlockPeerInfoStruct)
	blockCIDRMap = make(map[string]BlockCIDRInfoStruct)
	currentTimestamp, lastIPCleanTimestamp = 100, 0
	current := map[string]IPInfoStruct{
		"192.0.2.1":    {Port: map[int]bool{1: true, 2: true}},
		"198.51.100.1": {Port: map[int]bool{3: true}, TorrentUploaded: map[string]int64{"hash": 10 << 20}},
	}
	previous := make(map[string]IPInfoStruct)
	if count := CheckAllIP(current, previous); count != 1 {
		t.Fatalf("first cycle blocks=%d, want only the multi-port IP", count)
	}
	if _, exists := blockPeerMap["198.51.100.1"]; exists {
		t.Fatal("first observation must not be treated as upload delta")
	}
	if previous["198.51.100.1"].TorrentUploaded["hash"] != 10<<20 {
		t.Fatal("cold start did not establish upload baseline")
	}
	current["198.51.100.1"].TorrentUploaded["hash"] = 13 << 20
	if previous["198.51.100.1"].TorrentUploaded["hash"] != 10<<20 {
		t.Fatal("baseline aliases current counters")
	}
	currentTimestamp = 102
	if count := CheckAllIP(current, previous); count != 1 {
		t.Fatalf("second cycle blocks=%d, want the upload-delta IP", count)
	}
	if _, exists := blockPeerMap["198.51.100.1"]; !exists {
		t.Fatal("upload delta was not blocked after cold start")
	}
}
