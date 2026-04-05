package main

import (
	"encoding/json"
	"github.com/tidwall/jsonc"
	"net"
	"sync"
	"sync/atomic"
)

type SyncServer_ConfigStruct struct {
	Interval    uint32              `json:"interval"`
	Status      string              `json:"status"`
	BlockIPRule map[string][]string `json:"blockIPRule"`
}
type SyncServer_SubmitStruct struct {
	Version    uint32                       `json:"version"`
	Timestamp  int64                        `json:"timestamp"`
	Token      string                       `json:"token"`
	TorrentMap map[string]TorrentInfoStruct `json:"torrentMap"`
}

type SyncServer_RuleStruct struct {
	Net    *net.IPNet
	Reason string
}

var syncServer_isSubmiting atomic.Bool
var syncServer_lastSync int64 = 0
var syncServer_syncConfig = &SyncServer_ConfigStruct{
	Interval:    60,
	Status:      "",
	BlockIPRule: make(map[string][]string),
}
var syncServer_CompiledRules []SyncServer_RuleStruct
var ipBlockCIDRMapMutex sync.RWMutex

func SyncWithServer_PrepareJSON(torrentMap map[string]TorrentInfoStruct) (bool, string) {
	torrentMapMutex.RLock()
	syncJSON, err := json.Marshal(SyncServer_SubmitStruct{Version: 1, Timestamp: atomic.LoadInt64(&currentTimestamp), Token: config.SyncServerToken, TorrentMap: torrentMap})
	torrentMapMutex.RUnlock()
	if err != nil {
		Log("SyncWithServer_PrepareJSON", GetLangText("Error-GenJSON"), true, err.Error())
		return false, ""
	}

	return true, string(syncJSON)
}
func SyncWithServer_Submit(syncJSON string) bool {
	_, _, syncServerContent := Submit(config.SyncServerURL, syncJSON, false, false, nil)
	if syncServerContent == nil {
		Log("SyncWithServer", GetLangText("Error-FetchResponse2"), true)
		return false
	}

	// 最大 8MB.
	if len(syncServerContent) > 8388608 {
		Log("SyncWithServer", GetLangText("Error-LargeFile"), true)
		return false
	}

	var newConfig SyncServer_ConfigStruct
	if err := json.Unmarshal(jsonc.ToJSON(syncServerContent), &newConfig); err != nil {
		Log("SyncWithServer", GetLangText("Error-ParseConfig"), true, err.Error())
		return false
	}

	if newConfig.Status != "" {
		Log("SyncWithServer", GetLangText("Error-SyncWithServer_ServerError"), true, newConfig.Status)
		return false
	}

	var tmpSyncServerCompiledRules []SyncServer_RuleStruct

	for reason, ipArr := range newConfig.BlockIPRule {
		logReason := false

		for ipBlockListLineNum, ipBlockListLine := range ipArr {
			ipBlockListLine = ProcessRemark(ipBlockListLine)
			if ipBlockListLine == "" {
				Log("Debug-SyncWithServer_Compile", GetLangText("Error-Debug-EmptyLine"), false, ipBlockListLineNum)
				continue
			}

			Log("Debug-SyncWithServer_Compile", ":%d %s", false, ipBlockListLineNum, ipBlockListLine)
			cidr := ParseIPCIDR(ipBlockListLine)
			if cidr == nil {
				Log("SyncWithServer_Compile", GetLangText("Error-SyncWithServer_Compile"), true, ipBlockListLineNum, ipBlockListLine)
				continue
			}

			if !logReason {
				logReason = true
				Log("SyncWithServer", GetLangText("SyncWithServer_Compile-BlockByReason"), true, reason)
			}

			tmpSyncServerCompiledRules = append(tmpSyncServerCompiledRules, SyncServer_RuleStruct{Net: cidr, Reason: reason})
			Log("SyncWithServer_BlockCIDR", ":%d %s", false, ipBlockListLineNum, ipBlockListLine)
		}
	}

	ipBlockCIDRMapMutex.Lock()
	syncServer_CompiledRules = tmpSyncServerCompiledRules
	ipBlockCIDRMapMutex.Unlock()

	syncServer_syncConfig = &newConfig

	Log("Debug-SyncWithServer", GetLangText("Success-SyncWithServer"), true, len(syncServer_CompiledRules))
	return true
}

func SyncServer_CheckPeer(ipObj net.IP) (bool, string) {
	if ipObj == nil {
		return false, ""
	}

	ipBlockCIDRMapMutex.RLock()
	defer ipBlockCIDRMapMutex.RUnlock()

	for _, rule := range syncServer_CompiledRules {
		if rule.Net != nil && rule.Net.Contains(ipObj) {
			return true, "Bad-IP_FromSyncServer (" + rule.Reason + ")"
		}
	}

	return false, ""
}
func SyncWithServer_FullSubmit(syncJSON string) bool {
	syncStatus := SyncWithServer_Submit(syncJSON)
	syncServer_isSubmiting.Store(false)

	return syncStatus
}

func SyncWithServer() bool {
	if config.SyncServerURL == "" {
		ipBlockCIDRMapMutex.Lock()
		if len(syncServer_CompiledRules) > 0 {
			syncServer_CompiledRules = nil
		}
		ipBlockCIDRMapMutex.Unlock()
		
		syncServer_syncConfig = &SyncServer_ConfigStruct{
			Interval:    60,
			Status:      "",
			BlockIPRule: make(map[string][]string),
		}
		return true
	}

	currSyncConfig := syncServer_syncConfig
	if (atomic.LoadInt64(&syncServer_lastSync)+int64(currSyncConfig.Interval)) > atomic.LoadInt64(&currentTimestamp) || syncServer_isSubmiting.Load() {
		return true
	}

	Log("Debug-SyncWithServer", "In progress..", false)

	status, syncJSON := SyncWithServer_PrepareJSON(torrentMap)
	if !status {
		return false
	}

	atomic.StoreInt64(&syncServer_lastSync, atomic.LoadInt64(&currentTimestamp))

	if syncServer_isSubmiting.CompareAndSwap(false, true) {
		GoWithCrashLog("SyncWithServer_FullSubmit", func() {
			SyncWithServer_FullSubmit(syncJSON)
		})
	}

	return true
}
