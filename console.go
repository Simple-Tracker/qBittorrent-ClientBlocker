package main

import (
	"os"
	"time"
	"syscall"
	"runtime"
	"os/signal"
)

var loopTicker *time.Ticker
var currentTimestamp int64 = 0

func Task() {
	if config.ClientURL == "" {
		Log("Task", GetLangText("Error-Task_EmptyURL"), false)
		return
	}
	if !IsSupportClient() {
		Log("Task", GetLangText("Error-Task_NotSupportClient"), false, currentClientType)
		return
	}
	
	metadata := FetchMaindata()
	if metadata == nil {
		return
	}

	cleanCount := ClearBlockPeer()

	emptyHashCount := 0
	noLeechersCount := 0
	badTorrentInfoCount := 0
	ptTorrentCount := 0

	blockCount := 0
	ipBlockCount := 0
	badPeersCount := 0

	switch currentClientType {
		case "qBittorrent":
			metadata2 := metadata.(*qB_MainDataStruct)
			for torrentInfoHash, torrentInfo := range metadata2.Torrents {
				ProcessTorrent(torrentInfoHash, torrentInfo.Tracker, torrentInfo.NumLeechs, torrentInfo.TotalSize, nil, &emptyHashCount, &noLeechersCount, &badTorrentInfoCount, &ptTorrentCount, &blockCount, &ipBlockCount, &badPeersCount)
			}
		case "Transmission":
			metadata2 := metadata.(*Tr_TorrentsStruct)
			for _, torrentInfo := range metadata2.Torrents {
				// 手动判断有无 Peer 正在下载.
				var leecherCount int64 = 0
				for _, torrentPeer := range torrentInfo.Peers {
					if torrentPeer.IsUploading {
						leecherCount++
					}
				}

				tracker := ""
				if torrentInfo.Private {
					tracker = "Private"
				}

				ProcessTorrent(torrentInfo.InfoHash, tracker, leecherCount, torrentInfo.TotalSize, torrentInfo.Peers, &emptyHashCount, &noLeechersCount, &badTorrentInfoCount, &ptTorrentCount, &blockCount, &ipBlockCount, &badPeersCount)
			}
	}

	currentIPBlockCount := CheckAllIP(ipMap, lastIPMap)
	torrentBlockCount, torrentIPBlockCount := CheckAllTorrent(torrentMap, lastTorrentMap)
	blockCount += torrentBlockCount
	ipBlockCount += torrentIPBlockCount

	Log("Debug-Task_IgnoreEmptyHashCount", "%d", false, emptyHashCount)
	Log("Debug-Task_IgnoreNoLeechersCount", "%d", false, noLeechersCount)
	Log("Debug-Task_IgnorePTTorrentCount", "%d", false, ptTorrentCount)
	Log("Debug-Task_IgnoreBadTorrentInfoCount", "%d", false, badTorrentInfoCount)
	Log("Debug-Task_IgnoreBadPeersCount", "%d", false, badPeersCount)

	if cleanCount != 0 || blockCount != 0 {
		SubmitBlockPeer(blockPeerMap)
		if !config.IPUploadedCheck && len(ipBlockListCompiled) <= 0 && len(ipfilterFromURLCompiled) <= 0 {
			Log("Task", GetLangText("Task_BanInfo"), true, blockCount, len(blockPeerMap))
		} else {
			Log("Task", GetLangText("Task_BanInfoWithIP"), true, blockCount, len(blockPeerMap), currentIPBlockCount, ipBlockCount)
		}
	}
}
func GC() {
	ipMapGCCount := (len(ipMap) - 23333333)

	if ipMapGCCount > 0 {
		Log("GC", GetLangText("GC_IPMap"), true, ipMapGCCount)
		for ip, _ := range ipMap {
			ipMapGCCount--
			delete(ipMap, ip)
			if ipMapGCCount <= 0 {
				break
			}
		}
		runtime.GC()
	}

	for torrentInfoHash, torrentInfo := range torrentMap {
		torrentInfoGCCount := (len(torrentInfo.Peers) - 2333333)
		if torrentInfoGCCount > 0 {
			Log("GC", GetLangText("GC_TorrentMap"), true, torrentInfoHash, torrentInfoGCCount)
			for peerIP, _ := range torrentInfo.Peers {
				torrentInfoGCCount--
				delete(torrentMap[torrentInfoHash].Peers, peerIP)
				if torrentInfoGCCount <= 0 {
					break
				}
			}
			runtime.GC()
		}
	}
}
func RunConsole() {
	go WaitStop()
	if config.StartDelay > 0 {
		Log("RunConsole", GetLangText("RunConsole_StartDelay"), false, config.StartDelay)
		time.Sleep(time.Duration(config.StartDelay) * time.Second)
	}
	if !LoadInitConfig(true) {
		Log("RunConsole", GetLangText("RunConsole_AuthFailed"), true)
		os.Exit(1)
	}
	Log("RunConsole", GetLangText("RunConsole_ProgramHasStarted"), true)
	loopTicker = time.NewTicker(time.Duration(config.Interval) * time.Second)
	for ; true; <- loopTicker.C {
		currentTimestamp = time.Now().Unix()
		LoadInitConfig(false)
		Task()
		GC()
	}
}
func WaitStop() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM)

	<-signalChan
		Log("WaitStop", GetLangText("WaitStop_Stoping"), true)
		if loopTicker != nil {
			loopTicker.Stop()
		}
		httpClient.CloseIdleConnections()
		httpClientWithoutCookie.CloseIdleConnections()
		StopServer()
		Platform_Stop()
		os.Exit(0)
}
