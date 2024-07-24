package main

import (
	"encoding/json"
	"net"

	"github.com/tidwall/jsonc"
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

var lastSync int64 = 0
var syncConfig = SyncServer_ConfigStruct{
	Interval:    60,
	Status:      "",
	BlockIPRule: make(map[string][]string),
}
var ipBlockCIDRMapFromSyncServerCompiled = make(map[string]*net.IPNet)

func SyncWithServer() bool {
	if config.SyncServerURL == "" || (lastSync+int64(syncConfig.Interval)) > CurrentTimestamp {
		return true
	}

	Log("Debug-SyncWithServer", "In progress..", false)

	syncJSON, err := json.Marshal(SyncServer_SubmitStruct{Version: 1, Timestamp: CurrentTimestamp, Token: config.SyncServerToken, TorrentMap: torrentMap})
	if err != nil {
		Log("SyncWithServer", GetLangText("Error-GenJSON"), true, err.Error())
		return false
	}

	lastSync = CurrentTimestamp

	_, _, syncServerContent := Submit(config.SyncServerURL, string(syncJSON), false, false, nil)
	if syncServerContent == nil {
		Log("SyncWithServer", GetLangText("Error-FetchResponse"), true)
		return false
	}

	// Max 8MB.
	if len(syncServerContent) > 8388608 {
		Log("SyncWithServer", GetLangText("Error-LargeFile"), true)
		return false
	}

	if err := json.Unmarshal(jsonc.ToJSON(syncServerContent), &syncConfig); err != nil {
		Log("SyncWithServer", GetLangText("Error-ParseConfig"), true, err.Error())
		return false
	}

	if syncConfig.Status != "" {
		Log("SyncWithServer", GetLangText("Error-SyncWithServer_ServerError"), true, syncConfig.Status)
		return false
	}

	tmpIPBlockCIDRMapFromSyncServerCompiled := make(map[string]*net.IPNet)

	for reason, ipArr := range syncConfig.BlockIPRule {
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
