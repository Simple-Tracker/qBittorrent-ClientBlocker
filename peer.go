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
	var blockPeerPortMap map[int]bool
	var lastPeerID string
	var lastPeerClient string
	var torrentDownloaded map[string]int64
	var torrentUploaded map[string]int64
	var torrentDownloadedRaw map[string]int64
	var torrentUploadedRaw map[string]int64

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

	if peerID == "" {
		peerID = lastPeerID
	}
	if peerClient == "" {
		peerClient = lastPeerClient
	}

	// 使用 delta-based 累加处理流量统计.
	if torrentInfoHash != "" {
		// 计算下载下载量差额.
		downloadedDelta := peerDownloaded - torrentDownloadedRaw[torrentInfoHash]
		if peerDownloaded < torrentDownloadedRaw[torrentInfoHash] {
			downloadedDelta = peerDownloaded
		}
		torrentDownloaded[torrentInfoHash] += downloadedDelta
		torrentDownloadedRaw[torrentInfoHash] = peerDownloaded

		// 计算上传下载量差额.
		uploadedDelta := peerUploaded - torrentUploadedRaw[torrentInfoHash]
		if peerUploaded < torrentUploadedRaw[torrentInfoHash] {
			uploadedDelta = peerUploaded
		}
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
	}
	blockPeerMapMutex.Unlock()

	AddBlockCIDR(peerIP, ParseIPCIDRByConfig(peerIP))

	if config.ExecCommand_Ban != "" {
		execCommand_Ban := config.ExecCommand_Ban
		execCommand_Ban = strings.Replace(execCommand_Ban, "{peerIP}", peerIP, -1)
		execCommand_Ban = strings.Replace(execCommand_Ban, "{peerPort}", strconv.Itoa(peerPort), -1)
		execCommand_Ban = strings.Replace(execCommand_Ban, "{torrentInfoHash}", torrentInfoHash, -1)
		status, out, err := ExecCommand(execCommand_Ban)

		if status {
			Log("AddBlockPeer", GetLangText("Success-ExecCommand"), true, out)
		} else {
			Log("AddBlockPeer", GetLangText("Failed-ExecCommand"), true, out, err)
		}
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
	if (blockPeerMap != nil && config.CleanInterval == 0) || (lastCleanTimestamp+int64(config.CleanInterval) < currentTimestamp) {
		blockPeerMapMutex.Lock()
		blockCIDRMapMutex.Lock()
		for peerIP, peerInfo := range blockPeerMap {
			if currentTimestamp > (peerInfo.Timestamp + int64(config.BanTime)) {
				peerNet := ParseIPCIDRByConfig(peerIP)

				if peerNet != nil {
					peerNetStr := peerNet.String()
					if blockCIDRInfo, exist := blockCIDRMap[peerNetStr]; exist {
						if blockCIDRInfo.Timestamp > peerInfo.Timestamp {
							peerInfo.Timestamp = blockCIDRInfo.Timestamp
							blockPeerMap[peerIP] = peerInfo
							continue
						}

						delete(blockCIDRInfo.IPs, peerIP)
						if len(blockCIDRInfo.IPs) <= 0 {
							delete(blockCIDRMap, peerNetStr)
						} else {
							blockCIDRMap[peerNetStr] = blockCIDRInfo
						}
					}
				}

				cleanCount++
				delete(blockPeerMap, peerIP)

				if config.ExecCommand_Unban != "" {
					for peerPort := range peerInfo.Port {
						execCommandUnban := config.ExecCommand_Unban
						execCommandUnban = strings.Replace(execCommandUnban, "{peerIP}", peerIP, -1)
						execCommandUnban = strings.Replace(execCommandUnban, "{peerPort}", strconv.Itoa(peerPort), -1)
						execCommandUnban = strings.Replace(execCommandUnban, "{torrentInfoHash}", peerInfo.InfoHash, -1)
						execCommands = append(execCommands, execCommandUnban)
					}
				}
			}
		}
		blockCIDRMapMutex.Unlock()
		blockPeerMapMutex.Unlock()
		if cleanCount != 0 {
			lastCleanTimestamp = currentTimestamp
			Log("ClearBlockPeer", GetLangText("Success-ClearBlockPeer"), true, cleanCount)
		}
	}

	for _, command := range execCommands {
		status, out, err := execPeerCommand(command)
		if status {
			Log("AddBlockPeer", GetLangText("Success-ExecCommand"), true, out)
		} else {
			Log("AddBlockPeer", GetLangText("Failed-ExecCommand"), true, out, err)
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
				Log("MatchBlockList_PeerClient", GetLangText("Error-MatchRegexpErr"), true, err.Error())
			} else if isMatchPeerClient {
				return true
			}
		}

		if peerID != "" {
			isMatchPeerID, err := blockRegex.MatchString(peerID)

			if err != nil {
				Log("MatchBlockList_PeerID", GetLangText("Error-MatchRegexpErr"), true, err.Error())
			} else if isMatchPeerID {
				return true
			}
		}
	}

	return false
}

// CheckPeer 对单个 Peer 进行完整检查.
func CheckPeer(peerIP string, peerPort int, peerID, peerClient string, peerDlSpeed, peerUpSpeed int64, peerProgress float64, peerDownloaded, peerUploaded int64, torrentInfoHash string, torrentTotalSize int64) (int, *net.IPNet) {
	if peerIP == "" || CheckPrivateIP(peerIP) {
		return -1, nil
	}

	if IsBlockedPeer(peerIP, peerPort, true) {
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

	for _, port := range config.PortBlockList {
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
		return -2, peerNet
	}

	ignoreByDownloaded := false
	if !config.IgnoreEmptyPeer || hasPeerClient {
		if config.IgnoreByDownloaded > 0 && (peerDownloaded/1024/1024) >= int64(config.IgnoreByDownloaded) {
			ignoreByDownloaded = true
		}
		if !ignoreByDownloaded && IsProgressNotMatchUploaded(torrentTotalSize, peerProgress, peerUploaded) {
			Log("CheckPeer_AddBlockPeer (Bad-Progress_Uploaded)", "%s:%d %s|%s (TorrentInfoHash: %s, TorrentTotalSize: %.2f MB, PeerDlSpeed: %.2f MB/s, PeerUpSpeed: %.2f MB/s, Progress: %.2f%%, Downloaded: %.2f MB, Uploaded: %.2f MB)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, (float64(torrentTotalSize) / 1024 / 1024), (float64(peerDlSpeed) / 1024 / 1024), (float64(peerUpSpeed) / 1024 / 1024), (peerProgress * 100), (float64(peerDownloaded) / 1024 / 1024), (float64(peerUploaded) / 1024 / 1024))
			AddBlockPeer("CheckPeer", "Bad-Progress_Uploaded", peerIP, peerPort, torrentInfoHash, peerID, peerClient, peerDownloaded, peerUploaded)
			return 1, peerNet
		}
	}

	if (config.IgnoreEmptyPeer && !hasPeerClient) || ignoreByDownloaded {
		return -2, peerNet
	}

	return 0, peerNet
}

// ProcessPeer 处理单个 Peer 的分析任务.
func ProcessPeer(peer *Peer, torrentInfoHash string, torrentTotalSize int64, blockCount *int, ipBlockCount *int, badPeersCount *int, emptyPeersCount *int) {
	peerIP := ProcessIP(peer.IP)
	peerStatus, peerNet := CheckPeer(peerIP, peer.Port, peer.ID, peer.Client, peer.DlSpeed, peer.UpSpeed, peer.Progress, peer.Downloaded, peer.Uploaded, torrentInfoHash, torrentTotalSize)
	if config.Debug_CheckPeer {
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
		if peerNet == nil {
			AddIPInfo(nil, peerIP, peer.Port, torrentInfoHash, peer.Downloaded, peer.Uploaded)
			AddTorrentInfo(torrentInfoHash, torrentTotalSize, nil, peerIP, peer.Port, peer.Progress, peer.Downloaded, peer.Uploaded, peer.ID, peer.Client)
		} else {
			AddIPInfo(peerNet, peerNet.String(), peer.Port, torrentInfoHash, peer.Downloaded, peer.Uploaded)
			AddTorrentInfo(torrentInfoHash, torrentTotalSize, peerNet, peerNet.String(), peer.Port, peer.Progress, peer.Downloaded, peer.Uploaded, peer.ID, peer.Client)
		}
	}
}
