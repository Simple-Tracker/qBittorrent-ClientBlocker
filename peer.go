package main

import (
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/dlclark/regexp2"
)

type BlockPeerInfoStruct struct {
	Timestamp            int64
	Module               string
	Reason               string
	Port                 map[int]bool
	InfoHash             string
	ID                   string
	Client               string
	Downloaded           int64
	Uploaded             int64
	TorrentDownloaded    map[string]int64
	TorrentUploaded      map[string]int64
	TorrentDownloadedRaw map[string]int64
	TorrentUploadedRaw   map[string]int64
	trafficCounters      map[string]map[int]PeerTrafficCounter
}
type BlockCIDRInfoStruct struct {
	Timestamp int64
	Net       *net.IPNet
	IPs       map[string]bool
}

var lastCleanTimestamp int64 = 0
var blockPeerMap = make(map[string]BlockPeerInfoStruct)
var blockCIDRMap = make(map[string]BlockCIDRInfoStruct)
var blockPeerMapMutex sync.RWMutex
var blockCIDRMapMutex sync.RWMutex
var execPeerCommand = ExecCommand

// AddBlockPeer 将 Peer 添加到封禁列表.
func AddBlockPeer(module string, reason string, peerIP string, peerPort int, torrentInfoHash string, peerID string, peerClient string, peerDownloaded int64, peerUploaded int64) {
	// 封禁名单只保存实际 IP；CIDR 规则由 blockCIDRMap 单独维护。
	if net.ParseIP(peerIP) == nil {
		LogError("AddBlockPeer", "Invalid peer IP: %q", true, peerIP)
		return
	}

	var blockPeerPortMap map[int]bool
	var lastPeerID string
	var lastPeerClient string
	var torrentDownloaded map[string]int64
	var torrentUploaded map[string]int64
	var torrentDownloadedRaw map[string]int64
	var torrentUploadedRaw map[string]int64
	var trafficCounters map[string]map[int]PeerTrafficCounter

	blockPeerMapMutex.Lock()
	if blockPeer, exist := blockPeerMap[peerIP]; !exist {
		blockPeerPortMap = make(map[int]bool)
		torrentDownloaded = make(map[string]int64)
		torrentUploaded = make(map[string]int64)
		torrentDownloadedRaw = make(map[string]int64)
		torrentUploadedRaw = make(map[string]int64)
	} else {
		blockPeerPortMap = blockPeer.Port
		lastPeerID = blockPeer.ID
		lastPeerClient = blockPeer.Client
		torrentDownloaded = blockPeer.TorrentDownloaded
		torrentUploaded = blockPeer.TorrentUploaded
		torrentDownloadedRaw = blockPeer.TorrentDownloadedRaw
		torrentUploadedRaw = blockPeer.TorrentUploadedRaw
		trafficCounters = blockPeer.trafficCounters
		if torrentDownloaded == nil {
			torrentDownloaded = make(map[string]int64)
		}
		if torrentUploaded == nil {
			torrentUploaded = make(map[string]int64)
		}
		if torrentDownloadedRaw == nil {
			torrentDownloadedRaw = make(map[string]int64)
		}
		if torrentUploadedRaw == nil {
			torrentUploadedRaw = make(map[string]int64)
		}
	}
	if trafficCounters == nil {
		trafficCounters = make(map[string]map[int]PeerTrafficCounter)
	}

	observedPeerID := peerID
	if peerID == "" {
		peerID = lastPeerID
	}
	if peerClient == "" {
		peerClient = lastPeerClient
	}

	// 使用 delta-based 累加处理流量统计.
	if torrentInfoHash != "" {
		downloadedDelta, uploadedDelta := blockedPeerCounterDelta(trafficCounters, torrentInfoHash, peerPort, peerDownloaded, peerUploaded, observedPeerID)
		torrentDownloaded[torrentInfoHash] += downloadedDelta
		torrentDownloadedRaw[torrentInfoHash] = peerDownloaded
		torrentUploaded[torrentInfoHash] += uploadedDelta
		torrentUploadedRaw[torrentInfoHash] = peerUploaded
	} else {
		// 全局增量处理.
		torrentDownloaded["__global__"] += peerDownloaded
		torrentUploaded["__global__"] += peerUploaded
	}

	// 汇总计算.
	var totalDownloaded int64 = 0
	var totalUploaded int64 = 0
	for _, v := range torrentDownloaded {
		totalDownloaded += v
	}
	for _, v := range torrentUploaded {
		totalUploaded += v
	}

	blockPeerPortMap[peerPort] = true
	blockPeerMap[peerIP] = BlockPeerInfoStruct{
		Timestamp:            currentTimestamp,
		Module:               module,
		Reason:               reason,
		Port:                 blockPeerPortMap,
		InfoHash:             torrentInfoHash,
		ID:                   peerID,
		Client:               peerClient,
		Downloaded:           totalDownloaded,
		Uploaded:             totalUploaded,
		TorrentDownloaded:    torrentDownloaded,
		TorrentUploaded:      torrentUploaded,
		TorrentDownloadedRaw: torrentDownloadedRaw,
		TorrentUploadedRaw:   torrentUploadedRaw,
		trafficCounters:      trafficCounters,
	}
	blockPeerMapMutex.Unlock()

	AddBlockCIDR(peerIP, ParseIPCIDRByConfig(peerIP))
	// 已有 IP 也可能新增端口或更新统计；同步接口会按 IP 合并为最终状态。
	WebUI_RecordBlockPeerAdded(peerIP)

	if ConfigSnapshot().ExecCommand_Ban != "" {
		execCommand_Ban := ConfigSnapshot().ExecCommand_Ban
		execCommand_Ban = strings.Replace(execCommand_Ban, "{peerIP}", peerIP, -1)
		execCommand_Ban = strings.Replace(execCommand_Ban, "{peerPort}", strconv.Itoa(peerPort), -1)
		execCommand_Ban = strings.Replace(execCommand_Ban, "{torrentInfoHash}", torrentInfoHash, -1)
		status, out, err := execPeerCommand(execCommand_Ban)

		if status {
			Log("AddBlockPeer", GetLangText("Success-ExecCommand"), true, out)
		} else {
			LogError("AddBlockPeer", GetLangText("Failed-ExecCommand"), true, out, err)
		}
	}
}

func blockedPeerCounterDelta(counters map[string]map[int]PeerTrafficCounter, hash string, port int, downloaded, uploaded int64, peerIDs ...string) (int64, int64) {
	if counters[hash] == nil {
		counters[hash] = make(map[int]PeerTrafficCounter)
	}
	previous, exist := counters[hash][port]
	if !exist {
		previous.Downloaded, previous.Uploaded = -1, -1
	}
	previous, _ = AlignPeerTrafficCounter(previous, peerIDs...)
	// 暂时未知的计数不能覆盖上次有效基线，否则恢复上报时会重复累计。
	current := previous
	if !exist || downloaded >= 0 {
		current.Downloaded = downloaded
	}
	if !exist || uploaded >= 0 {
		current.Uploaded = uploaded
	}
	current.LastSeen = currentTimestamp
	counters[hash][port] = current
	return CounterDelta(downloaded, previous.Downloaded), CounterDelta(uploaded, previous.Uploaded)
}

// SeedBlockedPeerCounters 为已经计入封禁总量的统计快照安装连接基线。
// 调用方提供持锁读取的快照；此处只获取封禁锁并复制，避免反向获取统计锁。
func SeedBlockedPeerCounters(peerIP string, counters map[string]map[int]PeerTrafficCounter) {
	blockPeerMapMutex.Lock()
	defer blockPeerMapMutex.Unlock()
	peer, exist := blockPeerMap[peerIP]
	if !exist {
		return
	}
	if peer.trafficCounters == nil {
		peer.trafficCounters = make(map[string]map[int]PeerTrafficCounter)
	}
	for hash, ports := range counters {
		if peer.trafficCounters[hash] == nil {
			peer.trafficCounters[hash] = make(map[int]PeerTrafficCounter, len(ports))
		}
		delete(peer.trafficCounters[hash], -1)
		for port, counter := range ports {
			peer.trafficCounters[hash][port] = counter
		}
	}
	blockPeerMap[peerIP] = peer
}

// UpdateBlockedPeerTraffic 只更新实际观测到的流量，不重复执行封禁操作或修改封禁原因。
func UpdateBlockedPeerTraffic(peerIP string, peerPort int, torrentInfoHash string, peerDownloaded, peerUploaded int64, peerIDs ...string) {
	if torrentInfoHash == "" {
		return
	}
	blockPeerMapMutex.Lock()
	peer, exist := blockPeerMap[peerIP]
	if !exist {
		blockPeerMapMutex.Unlock()
		return
	}
	if peer.TorrentDownloaded == nil {
		peer.TorrentDownloaded = make(map[string]int64)
	}
	if peer.TorrentUploaded == nil {
		peer.TorrentUploaded = make(map[string]int64)
	}
	if peer.TorrentDownloadedRaw == nil {
		peer.TorrentDownloadedRaw = make(map[string]int64)
	}
	if peer.TorrentUploadedRaw == nil {
		peer.TorrentUploadedRaw = make(map[string]int64)
	}
	if peer.trafficCounters == nil {
		peer.trafficCounters = make(map[string]map[int]PeerTrafficCounter)
	}
	downloadedDelta, uploadedDelta := blockedPeerCounterDelta(peer.trafficCounters, torrentInfoHash, peerPort, peerDownloaded, peerUploaded, peerIDs...)
	peer.TorrentDownloaded[torrentInfoHash] += downloadedDelta
	peer.TorrentUploaded[torrentInfoHash] += uploadedDelta
	peer.TorrentDownloadedRaw[torrentInfoHash] = peerDownloaded
	peer.TorrentUploadedRaw[torrentInfoHash] = peerUploaded
	peer.Downloaded += downloadedDelta
	peer.Uploaded += uploadedDelta
	blockPeerMap[peerIP] = peer
	blockPeerMapMutex.Unlock()
	if downloadedDelta != 0 || uploadedDelta != 0 {
		WebUI_RecordBlockPeerAdded(peerIP)
	}
}

// AddBlockCIDR 将 CIDR 网段添加到封禁列表.
func AddBlockCIDR(peerIP string, peerNet *net.IPNet) {
	if peerNet == nil {
		return
	}

	peerNetStr := peerNet.String()
	var blockIPsMap map[string]bool
	blockCIDRMapMutex.Lock()
	if blockCIDRInfo, exist := blockCIDRMap[peerNetStr]; !exist {
		blockIPsMap = make(map[string]bool)
		blockIPsMap[peerIP] = true
	} else {
		blockIPsMap = blockCIDRMap[peerNetStr].IPs
		if _, exist := blockCIDRInfo.IPs[peerIP]; !exist {
			blockIPsMap[peerIP] = true
		}
	}

	blockCIDRMap[peerNetStr] = BlockCIDRInfoStruct{Timestamp: currentTimestamp, Net: peerNet, IPs: blockIPsMap}
	blockCIDRMapMutex.Unlock()
}

// ClearBlockPeer 根据过期时间清理封禁列表.
func ClearBlockPeer() int {
	cleanCount := 0
	execCommands := []string{}
	removedPeerIPs := []string{}
	if (blockPeerMap != nil && ConfigSnapshot().CleanInterval == 0) || (lastCleanTimestamp+int64(ConfigSnapshot().CleanInterval) < currentTimestamp) {
		blockPeerMapMutex.Lock()
		blockCIDRMapMutex.Lock()
		// 使用封禁时保存的成员关系续期；热重载后的掩码可能已不属于原网段。
		for _, blockCIDRInfo := range blockCIDRMap {
			for peerIP := range blockCIDRInfo.IPs {
				if peerInfo, exist := blockPeerMap[peerIP]; exist && currentTimestamp > peerInfo.Timestamp+int64(ConfigSnapshot().BanTime) && blockCIDRInfo.Timestamp > peerInfo.Timestamp {
					peerInfo.Timestamp = blockCIDRInfo.Timestamp
					blockPeerMap[peerIP] = peerInfo
				}
			}
		}
		for peerIP, peerInfo := range blockPeerMap {
			if currentTimestamp > (peerInfo.Timestamp + int64(ConfigSnapshot().BanTime)) {
				cleanCount++
				delete(blockPeerMap, peerIP)
				removedPeerIPs = append(removedPeerIPs, peerIP)

				if ConfigSnapshot().ExecCommand_Unban != "" {
					for peerPort := range peerInfo.Port {
						execCommandUnban := ConfigSnapshot().ExecCommand_Unban
						execCommandUnban = strings.Replace(execCommandUnban, "{peerIP}", peerIP, -1)
						execCommandUnban = strings.Replace(execCommandUnban, "{peerPort}", strconv.Itoa(peerPort), -1)
						execCommandUnban = strings.Replace(execCommandUnban, "{torrentInfoHash}", peerInfo.InfoHash, -1)
						execCommands = append(execCommands, execCommandUnban)
					}
				}
			}
		}
		for peerNetStr, blockCIDRInfo := range blockCIDRMap {
			for peerIP := range blockCIDRInfo.IPs {
				if _, exist := blockPeerMap[peerIP]; !exist {
					delete(blockCIDRInfo.IPs, peerIP)
				}
			}
			if len(blockCIDRInfo.IPs) == 0 {
				delete(blockCIDRMap, peerNetStr)
			}
		}
		blockCIDRMapMutex.Unlock()
		blockPeerMapMutex.Unlock()
		for _, peerIP := range removedPeerIPs {
			WebUI_RecordBlockPeerRemoved(peerIP)
		}
		lastCleanTimestamp = currentTimestamp
		if cleanCount != 0 {
			Log("ClearBlockPeer", GetLangText("Success-ClearBlockPeer"), true, cleanCount)
		}
	}

	for _, command := range execCommands {
		status, out, err := execPeerCommand(command)
		if status {
			Log("AddBlockPeer", GetLangText("Success-ExecCommand"), true, out)
		} else {
			LogError("AddBlockPeer", GetLangText("Failed-ExecCommand"), true, out, err)
		}
	}

	return cleanCount
}

// IsBlockedPeer 检查 Peer 是否已被封禁.
func IsBlockedPeer(peerIP string, peerPort int, updateTimestamp bool) bool {
	blockPeerMapMutex.RLock()
	blockPeer, exist := blockPeerMap[peerIP]
	blockPeerMapMutex.RUnlock()

	if exist {
		if IsBanPort() {
			if _, exist1 := blockPeer.Port[-1]; !exist1 {
				if _, exist2 := blockPeer.Port[peerPort]; !exist2 {
					return false
				}
			}
		}

		if updateTimestamp {
			blockPeerMapMutex.Lock()
			if bp, exist := blockPeerMap[peerIP]; exist {
				bp.Timestamp = currentTimestamp
				blockPeerMap[peerIP] = bp
			}
			blockPeerMapMutex.Unlock()
		}

		return true
	}

	return false
}

// MatchBlockList 检查 Peer 是否匹配关键词黑名单.
func MatchBlockList(blockRegex *regexp2.Regexp, peerIP string, peerPort int, peerID string, peerClient string) bool {
	if blockRegex != nil {
		if peerClient != "" {
			isMatchPeerClient, err := blockRegex.MatchString(peerClient)

			if err != nil {
				LogError("MatchBlockList_PeerClient", GetLangText("Error-MatchRegexpErr"), true, err.Error())
			} else if isMatchPeerClient {
				return true
			}
		}

		if peerID != "" {
			isMatchPeerID, err := blockRegex.MatchString(peerID)

			if err != nil {
				LogError("MatchBlockList_PeerID", GetLangText("Error-MatchRegexpErr"), true, err.Error())
			} else if isMatchPeerID {
				return true
			}
		}
	}

	return false
}

func IsTrackedPeer(peerIP string, peerPort int, torrentInfoHash string) bool {
	ipMapMutex.RLock()
	_, tracked := ipMap[peerIP].TorrentPeers[torrentInfoHash][peerPort]
	ipMapMutex.RUnlock()
	if tracked {
		return true
	}
	torrentMapMutex.RLock()
	_, tracked = torrentMap[torrentInfoHash].Peers[peerIP].Connections[peerPort]
	torrentMapMutex.RUnlock()
	return tracked
}

// CheckPeer 对单个 Peer 进行完整检查.
func CheckPeer(peerIP string, peerPort int, peerID, peerClient string, peerDlSpeed, peerUpSpeed int64, peerProgress float64, peerDownloaded, peerUploaded int64, torrentInfoHash string, torrentTotalSize int64) (int, *net.IPNet) {
	if peerIP == "" || CheckPrivateIP(peerIP) {
		return -1, nil
	}

	if IsBlockedPeer(peerIP, peerPort, true) {
		UpdateBlockedPeerTraffic(peerIP, peerPort, torrentInfoHash, peerDownloaded, peerUploaded, peerID)
		Log("Debug-CheckPeer_IgnorePeer (Blocked)", "%s:%d %s|%s", false, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient))
		if peerPort == -1 {
			return 3, nil
		}
		return 2, nil
	}

	peerNet := ParseIPCIDRByConfig(peerIP)
	hasPeerClient := (peerID != "" || peerClient != "")

	if hasPeerClient {
		earlyStop := false
		blockListCompiled.Range(func(key, val any) bool {
			if MatchBlockList(val.(*regexp2.Regexp), peerIP, peerPort, peerID, peerClient) {
				Log("CheckPeer_AddBlockPeer (Bad-Client_Normal)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer("CheckPeer", "Bad-Client_Normal", peerIP, peerPort, torrentInfoHash, peerID, peerClient, peerDownloaded, peerUploaded)
				earlyStop = true
				return false
			}
			return true
		})

		if earlyStop {
			return 1, peerNet
		}
	}

	for _, port := range ConfigSnapshot().PortBlockList {
		if int(port) == peerPort {
			Log("CheckPeer_AddBlockPeer (Bad-Port)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
			AddBlockPeer("CheckPeer", "Bad-Port", peerIP, peerPort, torrentInfoHash, peerID, peerClient, peerDownloaded, peerUploaded)
			return 1, peerNet
		}
	}

	ip := net.ParseIP(peerIP)
	if ip == nil {
		Log("Debug-CheckPeer_AddBlockPeer (Bad-IP)", "%s:%d %s|%s (TorrentInfoHash: %s)", false, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
	} else {
		earlyStop := false
		ipBlockListCompiled.Range(func(_, v any) bool {
			if v == nil {
				return true
			}

			ipNet, ok := (v).(*net.IPNet)
			if !ok {
				return true
			}
			if ipNet.Contains(ip) {
				Log("CheckPeer_AddBlockPeer (Bad-IP_Normal)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer("CheckPeer", "Bad-IP_Normal", peerIP, -1, torrentInfoHash, peerID, peerClient, peerDownloaded, peerUploaded)
				earlyStop = true
				return false
			}

			return true
		})
		if earlyStop {
			return 3, peerNet
		}

		if isBlocked, reason := SyncServer_CheckPeer(ip); isBlocked {
			Log("CheckPeer_AddBlockPeer (Bad-IP_FromSyncServer)", "%s:%d %s|%s (TorrentInfoHash: %s, Reason: %s)", true, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, reason)
			AddBlockPeer("SyncServer", reason, peerIP, -1, torrentInfoHash, peerID, peerClient, peerDownloaded, peerUploaded)
			return 3, peerNet
		}
	}

	// BTN 规则检查.
	if isBlocked, banPort, reason := BTN_CheckPeer(peerIP, peerID, peerClient, peerPort); isBlocked {
		Log("CheckPeer_AddBlockPeer (Bad-IP_FromBTN)", "%s:%d %s|%s (TorrentInfoHash: %s, Reason: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, reason)
		AddBlockPeer("BTN", reason, peerIP, banPort, torrentInfoHash, peerID, peerClient, peerDownloaded, peerUploaded)
		if banPort == -1 {
			return 3, peerNet
		}
		return 1, peerNet
	}

	if IsMatchCIDR(peerNet) {
		Log("CheckPeer_AddBlockPeer (Bad-CIDR)", "%s:%d %s|%s (TorrentInfoHash: %s, PeerNet: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, peerNet.String())
		AddBlockPeer("CheckPeer", "Bad-CIDR", peerIP, peerPort, torrentInfoHash, peerID, peerClient, peerDownloaded, peerUploaded)
		return 1, peerNet
	}

	if peerDlSpeed <= 0 && peerUpSpeed <= 0 {
		cfg := ConfigSnapshot()
		ignored := (cfg.IgnoreEmptyPeer && !hasPeerClient) || (cfg.IgnoreByDownloaded > 0 && (peerDownloaded/1024/1024) >= int64(cfg.IgnoreByDownloaded))
		// 已观测连接的尾样本仍可能增加累计量；新出现的空闲连接继续忽略。
		if !ignored && IsTrackedPeer(peerIP, peerPort, torrentInfoHash) {
			return 0, peerNet
		}
		return -2, peerNet
	}

	ignoreByDownloaded := false
	if !ConfigSnapshot().IgnoreEmptyPeer || hasPeerClient {
		if ConfigSnapshot().IgnoreByDownloaded > 0 && (peerDownloaded/1024/1024) >= int64(ConfigSnapshot().IgnoreByDownloaded) {
			ignoreByDownloaded = true
		}
		if !ignoreByDownloaded && IsProgressNotMatchUploaded(torrentTotalSize, peerProgress, peerUploaded) {
			Log("CheckPeer_AddBlockPeer (Bad-Progress_Uploaded)", "%s:%d %s|%s (TorrentInfoHash: %s, TorrentTotalSize: %.2f MB, PeerDlSpeed: %.2f MB/s, PeerUpSpeed: %.2f MB/s, Progress: %.2f%%, Downloaded: %.2f MB, Uploaded: %.2f MB)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, (float64(torrentTotalSize) / 1024 / 1024), (float64(peerDlSpeed) / 1024 / 1024), (float64(peerUpSpeed) / 1024 / 1024), (peerProgress * 100), (float64(peerDownloaded) / 1024 / 1024), (float64(peerUploaded) / 1024 / 1024))
			AddBlockPeer("CheckPeer", "Bad-Progress_Uploaded", peerIP, peerPort, torrentInfoHash, peerID, peerClient, peerDownloaded, peerUploaded)
			return 1, peerNet
		}
	}

	if (ConfigSnapshot().IgnoreEmptyPeer && !hasPeerClient) || ignoreByDownloaded {
		return -2, peerNet
	}

	return 0, peerNet
}

// ProcessPeer 处理单个 Peer 的分析任务.
func ProcessPeer(peer *Peer, torrentInfoHash string, torrentTotalSize int64, blockCount *int, ipBlockCount *int, badPeersCount *int, emptyPeersCount *int) {
	peerIP := ProcessIP(peer.IP)
	peerStatus, peerNet := CheckPeer(peerIP, peer.Port, peer.ID, peer.Client, peer.DlSpeed, peer.UpSpeed, peer.Progress, peer.Downloaded, peer.Uploaded, torrentInfoHash, torrentTotalSize)
	if ConfigSnapshot().Debug_CheckPeer {
		Log("Debug-CheckPeer", "%s:%d %s|%s (TorrentInfoHash: %s, TorrentTotalSize: %.2f MB, PeerDlSpeed: %.2f MB/s, PeerUpSpeed: %.2f MB/s, Progress: %.2f%%, Downloaded: %.2f MB, Uploaded: %.2f MB, PeerStatus: %d)", false, peerIP, peer.Port, strconv.QuoteToASCII(peer.ID), strconv.QuoteToASCII(peer.Client), torrentInfoHash, (float64(torrentTotalSize) / 1024 / 1024), (float64(peer.DlSpeed) / 1024 / 1024), (float64(peer.UpSpeed) / 1024 / 1024), (peer.Progress * 100), (float64(peer.Downloaded) / 1024 / 1024), (float64(peer.Uploaded) / 1024 / 1024), peerStatus)
	}

	switch peerStatus {
	case 1:
		*blockCount++
	case 3:
		*ipBlockCount++
	case -1:
		*badPeersCount++
	case -2:
		*emptyPeersCount++
	case 0:
		// 保留实际 Peer 身份；网段汇总在统计判定阶段进行。
		AddIPInfo(peerNet, peerIP, peer.Port, torrentInfoHash, peer.Downloaded, peer.Uploaded, peer.ID)
		AddTorrentInfo(torrentInfoHash, torrentTotalSize, peerNet, peerIP, peer.Port, peer.Progress, peer.Downloaded, peer.Uploaded, peer.ID, peer.Client)
	}
}
