package main

import (
	"encoding/json"
	"github.com/tidwall/jsonc"
	"net"
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

var syncServer_isSubmiting bool = false
var syncServer_lastSync int64 = 0
var syncServer_syncConfig = SyncServer_ConfigStruct{
	Interval:    60,
	Status:      "",
	BlockIPRule: make(map[string][]string),
}
var ipBlockCIDRMapFromSyncServerCompiled = make(map[string]*net.IPNet)

func SyncWithServer_PrepareJSON(torrentMap map[string]TorrentInfoStruct) (bool, string) {
	syncJSON, err := json.Marshal(SyncServer_SubmitStruct{Version: 1, Timestamp: currentTimestamp, Token: config.SyncServerToken, TorrentMap: torrentMap})
	if err != nil {
		Log("SyncWithServer_PrepareJSON", GetLangText("Error-GenJSON"), true, err.Error())
		return false, ""
	}

	return true, string(syncJSON)
}
func SyncWithServer_Submit(syncJSON string) bool {
	_, _, syncServerContent := Submit(config.SyncServerURL, syncJSON, false, false, nil)
	syncServer_isSubmiting = false
	if syncServerContent == nil {
		Log("SyncWithServer", GetLangText("Error-FetchResponse2"), true)
		return false
	}

	// Max 8MB.
	if len(syncServerContent) > 8388608 {
		Log("SyncWithServer", GetLangText("Error-LargeFile"), true)
		return false
	}

	if err := json.Unmarshal(jsonc.ToJSON(syncServerContent), &syncServer_syncConfig); err != nil {
		Log("SyncWithServer", GetLangText("Error-ParseConfig"), true, err.Error())
		return false
	}

	if syncServer_syncConfig.Status != "" {
		Log("SyncWithServer", GetLangText("Error-SyncWithServer_ServerError"), true, syncServer_syncConfig.Status)
		return false
	}

	tmpIPBlockCIDRMapFromSyncServerCompiled := make(map[string]*net.IPNet)

	for reason, ipArr := range syncServer_syncConfig.BlockIPRule {
		logReason := false

		for ipBlockListLineNum, ipBlockListLine := range ipArr {
			ipBlockListLine = ProcessRemark(ipBlockListLine)
			if ipBlockListLine == "" {
				Log("Debug-SyncWithServer_Compile", GetLangText("Error-Debug-EmptyLine"), false, ipBlockListLineNum)
				continue
			}

			if cidr, exists := ipBlockCIDRMapFromSyncServerCompiled[ipBlockListLine]; exists {
				tmpIPBlockCIDRMapFromSyncServerCompiled[ipBlockListLine] = cidr
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

			tmpIPBlockCIDRMapFromSyncServerCompiled[ipBlockListLine] = cidr
			Log("SyncWithServer_BlockCIDR", ":%d %s", false, ipBlockListLineNum, ipBlockListLine)
		}
	}

	ipBlockCIDRMapFromSyncServerCompiled = tmpIPBlockCIDRMapFromSyncServerCompiled

	Log("Debug-SyncWithServer", GetLangText("Success-SyncWithServer"), true, len(ipBlockCIDRMapFromSyncServerCompiled))
	return true
}
func SyncWithServer_FullSubmit(syncJSON string) bool {
	syncServer_isSubmiting = true
	syncStatus := SyncWithServer_Submit(syncJSON)
	syncServer_isSubmiting = false

	return syncStatus
}
func SyncWithServer() bool {
	if config.SyncServerURL == "" || (syncServer_lastSync+int64(syncServer_syncConfig.Interval)) > currentTimestamp || syncServer_isSubmiting {
		return true
	}

	Log("Debug-SyncWithServer", "In progress..", false)

	status, syncJSON := SyncWithServer_PrepareJSON(torrentMap)
	if !status {
		return false
	}

	syncServer_lastSync = currentTimestamp

	go SyncWithServer_FullSubmit(syncJSON)

	return true
}
