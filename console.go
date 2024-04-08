package main

import (
	"os"
	"net"
	"time"
	"strings"
	"strconv"
	"runtime"
)

type IPInfoStruct struct {
	Port map[int]bool
	TorrentUploaded map[string]int64
}
type PeerInfoStruct struct {
	Port     map[int]bool
	Progress float64
	Uploaded int64
}
type TorrentInfoStruct struct {
	Size  int64
	Peers map[string]PeerInfoStruct
}
type BlockPeerInfoStruct struct {
	Timestamp int64
	Port      map[int]bool
}

var currentTimestamp int64 = 0
var lastCleanTimestamp int64 = 0
var lastIPCleanTimestamp int64 = 0
var lastTorrentCleanTimestamp int64 = 0
var ipMap = make(map[string]IPInfoStruct)
var torrentMap = make(map[string]TorrentInfoStruct)
var blockPeerMap = make(map[string]BlockPeerInfoStruct)
var blockCIDRMap = make(map[string]*net.IPNet)
var lastIPMap = make(map[string]IPInfoStruct)
var lastTorrentMap = make(map[string]TorrentInfoStruct)

func AddIPInfo(peerIP string, peerPort int, torrentInfoHash string, peerUploaded int64) {
	if !(config.MaxIPPortCount > 0 || (config.IPUploadedCheck && config.IPUpCheckIncrementMB > 0)) {
		return
	}

	var clientPortMap map[int]bool
	var clientTorrentUploadedMap map[string]int64
	if info, exist := ipMap[peerIP]; !exist {
		clientPortMap = make(map[int]bool)
		clientTorrentUploadedMap = make(map[string]int64)
	} else {
		clientPortMap = info.Port
		clientTorrentUploadedMap = info.TorrentUploaded
	}
	clientPortMap[peerPort] = true

	if oldPeerUploaded, exist := clientTorrentUploadedMap[torrentInfoHash]; (!exist || oldPeerUploaded <= peerUploaded) {
		clientTorrentUploadedMap[torrentInfoHash] = peerUploaded
	} else {
		clientTorrentUploadedMap[torrentInfoHash] += peerUploaded
	}

	ipMap[peerIP] = IPInfoStruct { Port: clientPortMap, TorrentUploaded: clientTorrentUploadedMap }
}
func AddTorrentInfo(torrentInfoHash string, torrentTotalSize int64, peerIP string, peerPort int, peerProgress float64, peerUploaded int64) {
	if !((config.IPUploadedCheck && config.IPUpCheckPerTorrentRatio > 0) || config.BanByRelativeProgressUploaded) {
		return
	}

	var peers map[string]PeerInfoStruct
	var peerPortMap map[int]bool
	if torrentInfo, exist := torrentMap[torrentInfoHash]; !exist {
		peers = make(map[string]PeerInfoStruct)
		peerPortMap = make(map[int]bool)
	} else {
		peers = torrentInfo.Peers
		if peerInfo, exist := peers[peerIP]; !exist {
			peerPortMap = make(map[int]bool)
		} else {
			peerPortMap = peerInfo.Port

			// 防止 Peer 在周期内以重新连接的方式清空实际上传量.
			if peerInfo.Uploaded > peerUploaded {
				peerUploaded += peerInfo.Uploaded
			}
		}
	}
	peerPortMap[peerPort] = true

	peers[peerIP] = PeerInfoStruct { Port: peerPortMap, Progress: peerProgress, Uploaded: peerUploaded }
	torrentMap[torrentInfoHash] = TorrentInfoStruct { Size: torrentTotalSize, Peers: peers }
}
func AddBlockPeer(peerIP string, peerPort int) {
	var blockPeerPortMap map[int]bool
	if blockPeer, exist := blockPeerMap[peerIP]; !exist {
		blockPeerPortMap = make(map[int]bool)
	} else {
		blockPeerPortMap = blockPeer.Port
	}

	blockPeerPortMap[peerPort] = true
	blockPeerMap[peerIP] = BlockPeerInfoStruct { Timestamp: currentTimestamp, Port: blockPeerPortMap }

	cidr := ""
	if IsIPv6(peerIP) {
		if config.BanIP6CIDR != "/128" {
			cidr = config.BanIP6CIDR
		}
	} else {
		if config.BanIPCIDR != "/32" {
			cidr = config.BanIPCIDR
		}
	}

	if cidr != "" {
		peerNet := ParseIP(peerIP + cidr)
		if peerNet != nil {
			peerNetStr := peerNet.String()
			blockCIDRMap[peerNetStr] = peerNet
		}
	}
}
func ClearBlockPeer() int {
	cleanCount := 0
	if config.CleanInterval == 0 || (lastCleanTimestamp + int64(config.CleanInterval) < currentTimestamp) {
		for clientIP, clientInfo := range blockPeerMap {
			if currentTimestamp > (clientInfo.Timestamp + int64(config.BanTime)) {
				cleanCount++
				delete(blockPeerMap, clientIP)

				cidr := ""
				if IsIPv6(clientIP) {
					if config.BanIP6CIDR != "/128" {
						cidr = config.BanIP6CIDR
					}
				} else {
					if config.BanIPCIDR != "/32" {
						cidr = config.BanIPCIDR
					}
				}

				if cidr != "" {
					peerNet := ParseIP(clientIP + cidr)
					if peerNet != nil {
						peerNetStr := peerNet.String()
						if _, exist := blockCIDRMap[peerNetStr]; !exist {
							delete(blockCIDRMap, peerNetStr)
						}
					}
				}
			}
		}
		if cleanCount != 0 {
			lastCleanTimestamp = currentTimestamp
			Log("ClearBlockPeer", GetLangText("Success-ClearBlockPeer"), true, cleanCount)
		}
	}

	return cleanCount
}
func IsBlockedPeer(peerIP string, peerPort int, updateTimestamp bool) bool {
	if blockPeer, exist := blockPeerMap[peerIP]; exist {
		if IsBanPort() {
			if _, exist1 := blockPeer.Port[-1]; !exist1 {
				if _, exist2 := blockPeer.Port[peerPort]; !exist2 {
					return false
				}
			}
		}

		if updateTimestamp {
			blockPeer.Timestamp = currentTimestamp
			blockPeerMap[peerIP] = blockPeer
		}

		return true
	}
	
	return false
}
func IsIPTooHighUploaded(ipInfo IPInfoStruct, lastIPInfo IPInfoStruct) int64 {
	var totalUploaded int64 = 0
	for torrentInfoHash, torrentUploaded := range ipInfo.TorrentUploaded {
		if config.IPUpCheckIncrementMB > 0 {
			if lastTorrentUploaded, exist := lastIPInfo.TorrentUploaded[torrentInfoHash]; !exist {
				totalUploaded += torrentUploaded
			} else {
				totalUploaded += (torrentUploaded - lastTorrentUploaded)
			}
		}
	}
	if config.IPUpCheckIncrementMB > 0 {
		var totalUploadedMB int64 = (totalUploaded / 1024 / 1024)
		if totalUploadedMB > int64(config.IPUpCheckIncrementMB) {
			return totalUploadedMB
		}
	}
	return 0
}
func IsProgressNotMatchUploaded(torrentTotalSize int64, clientProgress float64, clientUploaded int64) bool {
	if config.BanByProgressUploaded && torrentTotalSize > 0 && clientProgress >= 0 && clientUploaded > 0 {
		/*
		条件 1. 若客户端对 Peer 上传已大于等于 Torrnet 大小的 2%;
		条件 2. 但 Peer 报告进度乘以下载量再乘以一定防误判倍率, 却比客户端上传量还小;
		若满足以上条件, 则认为 Peer 是有问题的.
		e.g.:
		若 torrentTotalSize: 100GB, clientProgress: 1% (0.01), clientUploaded: 6GB, config.BanByPUStartPrecent: 2 (0.02), config.BanByPUAntiErrorRatio: 5;
		判断条件 1:
		torrentTotalSize * config.BanByPUStartPrecent = 100GB * 0.02 = 2GB, clientUploaded = 6GB >= 2GB
		满足此条件;
		判断条件 2:
		torrentTotalSize * clientProgress * config.BanByPUAntiErrorRatio = 100GB * 0.01 * 5 = 5GB, 5GB < clientUploaded = 6GB
		满足此条件;
		则该 Peer 将被封禁, 由于其报告进度为 1%, 算入 config.BanByPUAntiErrorRatio 滞后防误判倍率后为 5% (5GB), 但客户端实际却已上传 6GB.
		*/
		startUploaded := (float64(torrentTotalSize) * (config.BanByPUStartPrecent / 100))
		peerReportDownloaded := (float64(torrentTotalSize) * clientProgress)
		if (clientUploaded / 1024 / 1024) >= int64(config.BanByPUStartMB) && float64(clientUploaded) >= startUploaded && (peerReportDownloaded * config.BanByPUAntiErrorRatio) < float64(clientUploaded) {
			return true
		}
	}
	return false
}
func IsProgressNotMatchUploaded_Relative(torrentTotalSize int64, peerInfo PeerInfoStruct, lastPeerInfo PeerInfoStruct) int64 {
	// 若客户端对 Peer 上传已大于 0, 且相对上传量大于起始上传量, 则继续判断.
	var relativeUploaded int64 = (peerInfo.Uploaded - lastPeerInfo.Uploaded)
	if torrentTotalSize > 0 && peerInfo.Uploaded > 0 && (float64(relativeUploaded) / 1024 / 1024) > float64(config.BanByRelativePUStartMB) {
		relativeUploadedPrecent := (1 - (float64(lastPeerInfo.Uploaded) / float64(peerInfo.Uploaded)))
		// 若相对上传百分比大于起始百分比, 则继续判断.
		if relativeUploadedPrecent > (config.BanByRelativePUStartPrecent / 100) {
			// 若相对上传百分比大于 Peer 报告进度乘以一定防误判倍率, 则认为 Peer 是有问题的.
			var peerReportProgress float64 = 0
			if peerInfo.Progress > 0 {
				peerReportProgress = (1 - (lastPeerInfo.Progress / peerInfo.Progress))
			}
			if relativeUploadedPrecent > (peerReportProgress * config.BanByRelativePUAntiErrorRatio) {
				return relativeUploaded
			}
		}
	}
	return 0
}
func IsMatchCIDR(ip string) string {
	cidr := ""
	if IsIPv6(ip) {
		if config.BanIP6CIDR != "/128" {
			cidr = config.BanIP6CIDR
		}
	} else {
		if config.BanIPCIDR != "/32" {
			cidr = config.BanIPCIDR
		}
	}

	if cidr != "" {
		peerNet := ParseIP(ip + cidr)
		if peerNet != nil {
			peerNetStr := peerNet.String()
			if _, exist := blockCIDRMap[peerNetStr]; !exist {
				return peerNetStr
			}
		}
	}

	return ""
}
func CheckTorrent(torrentInfoHash string, tracker string, leecherCount int64) (int, interface{}) {
	if torrentInfoHash == "" {
		return -1, nil
	}
	if config.IgnorePTTorrent && tracker != "" {
		lowerTracker := strings.ToLower(tracker)
		if strings.Contains(lowerTracker, "?passkey=") || strings.Contains(lowerTracker, "?authkey=") || strings.Contains(lowerTracker, "?secure=") || randomStrRegexp.MatchString(lowerTracker) {
			return -4, nil
		}
	}
	if leecherCount <= 0 {
		return -2, nil
	}
	torrentPeers := FetchTorrentPeers(torrentInfoHash)
	if torrentPeers == nil {
		return -3, nil
	}
	return 0, torrentPeers
}
func CheckPeer(peerIP string, peerPort int, peerID string, peerClient string, peerProgress float64, peerUploaded int64, torrentInfoHash string, torrentTotalSize int64) int {
	hasPeerClient := (peerID != "" || peerClient != "")
	if (!config.IgnoreEmptyPeer && !hasPeerClient) || peerIP == "" || CheckPrivateIP(peerIP) {
		return -1
	}

	if IsBlockedPeer(peerIP, peerPort, true) {
		Log("Debug-CheckPeer_IgnorePeer (Blocked)", "%s:%d %s|%s", false, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient))
		/*
		if peerPort == -2 {
			return 4
		}
		*/
		if peerPort == -1 {
			return 3
		}
		return 2
	}

	peerNetStr := IsMatchCIDR(peerIP)
	if peerNetStr != "" {
		Log("CheckPeer_AddBlockPeer (Bad-CIDR)", "%s:%d (Net: %s)", false, peerIP, peerPort, peerNetStr)
		AddBlockPeer(peerIP, peerPort)
		return 1
	}

	if IsProgressNotMatchUploaded(torrentTotalSize, peerProgress, peerUploaded) {
		Log("CheckPeer_AddBlockPeer (Bad-Progress_Uploaded)", "%s:%d %s|%s (TorrentInfoHash: %s, TorrentTotalSize: %.2f MB, Progress: %.2f%%, Uploaded: %.2f MB)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, (float64(torrentTotalSize) / 1024 / 1024), (peerProgress * 100), (float64(peerUploaded) / 1024 / 1024))
		AddBlockPeer(peerIP, peerPort)
		return 1
	}

	if hasPeerClient {
		for _, v := range blockListCompiled {
			if v == nil {
				continue
			}
			if (peerClient != "" && v.MatchString(peerClient)) || (peerID != "" && v.MatchString(peerID)) {
				Log("CheckPeer_AddBlockPeer (Bad-Client)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, peerPort)
				return 1
			}
		}
	}

	ip := net.ParseIP(peerIP)
	if ip == nil {
		Log("Debug-CheckPeer_AddBlockPeer (Bad-IP)", "%s:%d %s|%s (TorrentInfoHash: %s)", false, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
	} else {
		for _, v := range ipBlockListCompiled {
			if v == nil {
				continue
			}
			if v.Contains(ip) {
				Log("CheckPeer_AddBlockPeer (Bad-IP_List)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, -1)
				return 3
			}
		}
		for _, v := range ipfilterCompiled {
			if v == nil {
				continue
			}
			if v.Contains(ip) {
				Log("CheckPeer_AddBlockPeer (Bad-IP_Filter)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, -1)
				return 3
			}
		}
	}

	return 0
}
func CheckAllIP(ipMap map[string]IPInfoStruct, lastIPMap map[string]IPInfoStruct) int {
	if (config.MaxIPPortCount > 0 || (config.IPUploadedCheck && config.IPUpCheckIncrementMB > 0)) && len(lastIPMap) > 0 && currentTimestamp > (lastIPCleanTimestamp + int64(config.IPUpCheckInterval)) {
		ipBlockCount := 0

		ipMapLoop:
		for ip, ipInfo := range ipMap {
			if IsBlockedPeer(ip, -1, true) || len(ipInfo.Port) <= 0 {
				continue
			}

			for port := range ipInfo.Port {
				if IsBlockedPeer(ip, port, true) {
					continue ipMapLoop
				}
			}

			if config.MaxIPPortCount > 0 {
				if len(ipInfo.Port) > int(config.MaxIPPortCount) {
					Log("CheckAllIP_AddBlockPeer (Too many ports)", "%s:%d", true, ip, -1)
					ipBlockCount++
					AddBlockPeer(ip, -1)
					continue
				}
			}

			if lastIPInfo, exist := lastIPMap[ip]; exist {
				if uploadDuring := IsIPTooHighUploaded(ipInfo, lastIPInfo); uploadDuring > 0 {
					Log("CheckAllIP_AddBlockPeer (Global-Too high uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, ip, -1, uploadDuring)
					ipBlockCount++
					AddBlockPeer(ip, -1)
				}
			}
		}

		lastIPCleanTimestamp = currentTimestamp
		DeepCopyIPMap(ipMap, lastIPMap)

		return ipBlockCount
	}

	return 0
}
func CheckAllTorrent(torrentMap map[string]TorrentInfoStruct, lastTorrentMap map[string]TorrentInfoStruct) (int, int) {
	if ((config.IPUploadedCheck && config.IPUpCheckPerTorrentRatio > 0) || config.BanByRelativeProgressUploaded) && len(lastTorrentMap) > 0 && currentTimestamp > (lastTorrentCleanTimestamp + int64(config.TorrentMapCleanInterval)) {
		blockCount := 0
		ipBlockCount := 0

		for torrentInfoHash, torrentInfo := range torrentMap {
			for peerIP, peerInfo := range torrentInfo.Peers {
				peerIP = ProcessIP(peerIP)

				if IsBlockedPeer(peerIP, -1, true) {
					continue
				}

				if config.IPUploadedCheck && config.IPUpCheckPerTorrentRatio > 0 {
					if float64(peerInfo.Uploaded) > (float64(torrentInfo.Size) * peerInfo.Progress * config.IPUpCheckPerTorrentRatio) {
						Log("CheckAllTorrent_AddBlockPeer (PerTorrent-Too high uploaded)", "%s:%d (TorrentSize: %.2f MB, Uploaded: %.2f MB)", true, peerIP, -1, (float64(torrentInfo.Size) / 1024 / 1024), (float64(peerInfo.Uploaded) / 1024 / 1024))
						ipBlockCount++
						AddBlockPeer(peerIP, -1)
						continue
					}
				}

				if config.BanByRelativeProgressUploaded {
					if lastPeerInfo, exist := lastTorrentMap[torrentInfoHash].Peers[peerIP]; exist {
						if uploadDuring := IsProgressNotMatchUploaded_Relative(torrentInfo.Size, peerInfo, lastPeerInfo); uploadDuring > 0 {
							for port := range peerInfo.Port {
								if IsBlockedPeer(peerIP, port, true) {
									continue
								}
								blockCount++
								Log("CheckAllTorrent_AddBlockPeer (Bad-Relative_Progress_Uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, peerIP, port, uploadDuring)
								AddBlockPeer(peerIP, port)
							}
							continue
						}
					}
				}
			}
		}

		lastTorrentCleanTimestamp = currentTimestamp
		DeepCopyTorrentMap(torrentMap, lastTorrentMap)

		return blockCount, ipBlockCount
	}

	return 0, 0
}
func Task() {
	if config.URL == "" {
		Log("Task", GetLangText("Error-Task_EmptyURL"), false)
		return
	}
	
	metadata := FetchMaindata()
	if metadata == nil {
		return
	}

	cleanCount := ClearBlockPeer()
	blockCount := 0
	ipBlockCount := 0
	emptyHashCount := 0
	noLeechersCount := 0
	ptTorrentCount := 0
	badTorrentInfoCount := 0
	badPeersCount := 0

	metadata2 := metadata.(*qB_MainDataStruct)

	for torrentInfoHash, torrentInfo := range metadata2.Torrents {
		torrentInfoHash = strings.ToLower(torrentInfoHash)
		torrentStatus, torrentPeersStruct := CheckTorrent(torrentInfoHash, torrentInfo.Tracker, torrentInfo.NumLeechs)
		if config.Debug_CheckTorrent {
			Log("Debug-CheckTorrent", "%s (Status: %d)", false, torrentInfoHash, torrentStatus)
		}
		skipSleep := false
		switch torrentStatus {
			case -1:
				skipSleep = true
				emptyHashCount++
			case -2:
				skipSleep = true
				noLeechersCount++
			case -3:
				badTorrentInfoCount++
			case -4:
				skipSleep = true
				ptTorrentCount++
			case 0:
				torrentPeers := torrentPeersStruct.(*qB_TorrentPeersStruct).Peers
				for _, peer := range torrentPeers {
					peer.IP = ProcessIP(peer.IP)
					peerStatus := CheckPeer(peer.IP, peer.Port, peer.Peer_ID_Client, peer.Client, peer.Progress, peer.Uploaded, torrentInfoHash, torrentInfo.TotalSize)

					if config.Debug_CheckPeer {
						Log("Debug-CheckPeer", "%s:%d %s|%s (Status: %d)", false, peer.IP, peer.Port, strconv.QuoteToASCII(peer.Peer_ID_Client), strconv.QuoteToASCII(peer.Client), peerStatus)
					}

					switch peerStatus {
						case 3:
							ipBlockCount++
						case 1:
							blockCount++
						case -1:
							badPeersCount++
						case 0:
							AddIPInfo(peer.IP, peer.Port, torrentInfoHash, peer.Uploaded)
							AddTorrentInfo(torrentInfoHash, torrentInfo.TotalSize, peer.IP, peer.Port, peer.Progress, peer.Uploaded)
					}
				}
		}
		if !skipSleep && config.SleepTime != 0 {
			time.Sleep(time.Duration(config.SleepTime) * time.Millisecond)
		}
	}

	currentIPBlockCount := CheckAllIP(ipMap, lastIPMap)
	ipBlockCount += currentIPBlockCount
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
		if !config.IPUploadedCheck && len(ipBlockListCompiled) <= 0 {
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
	if config.StartDelay > 0 {
		Log("RunConsole", GetLangText("RunConsole_StartDelay"), false, config.StartDelay)
		time.Sleep(time.Duration(config.StartDelay) * time.Second)
	}
	if !LoadInitConfig(true) {
		Log("RunConsole", GetLangText("RunConsole_AuthFailed"), true)
		os.Exit(1)
	}
	Log("RunConsole", GetLangText("RunConsole_ProgramHasStarted"), true)
	loopTicker := time.NewTicker(time.Duration(config.Interval) * time.Second)
	defer loopTicker.Stop()
	for ; true; <- loopTicker.C {
		currentTimestamp = time.Now().Unix()
		LoadInitConfig(false)
		Task()
		GC()
	}
}
