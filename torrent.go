package main

import (
	"net"
	"strings"
	"sync"
	"time"
)

type TorrentInfoStruct struct {
	Size  int64
	Peers map[string]PeerInfoStruct
}
type PeerInfoStruct struct {
	Net        *net.IPNet
	Port       map[int]bool
	Progress   float64
	Downloaded int64
	Uploaded   int64
	ID         string
	Client     string
}

var torrentMap = make(map[string]TorrentInfoStruct)
var lastTorrentMap = make(map[string]TorrentInfoStruct)
var torrentMapMutex sync.RWMutex
var lastTorrentMapMutex sync.RWMutex
var lastTorrentCleanTimestamp int64 = 0

// AddTorrentInfo 添加种子信息, 以便后续进行上传进度比分析.
func AddTorrentInfo(torrentInfoHash string, torrentTotalSize int64, cidr *net.IPNet, peerIP string, peerPort int, peerProgress float64, peerDownloaded int64, peerUploaded int64, peerID string, peerClient string) {
	if !((config.IPUploadedCheck && config.IPUpCheckPerTorrentRatio > 0) || config.BanByRelativeProgressUploaded || config.SyncServerURL != "") {
		return
	}

	var peers map[string]PeerInfoStruct
	var peerPortMap map[int]bool
	torrentMapMutex.Lock()
	if torrentInfo, exist := torrentMap[torrentInfoHash]; !exist {
		peers = make(map[string]PeerInfoStruct)
		peerPortMap = make(map[int]bool)
	} else {
		peers = torrentInfo.Peers
		if peerInfo, exist := peers[peerIP]; !exist {
			peerPortMap = make(map[int]bool)
		} else {
			peerPortMap = peerInfo.Port
		}
	}
	peerPortMap[peerPort] = true

	peers[peerIP] = PeerInfoStruct{Net: cidr, Port: peerPortMap, Progress: peerProgress, Downloaded: peerDownloaded, Uploaded: peerUploaded, ID: peerID, Client: peerClient}
	torrentMap[torrentInfoHash] = TorrentInfoStruct{Size: torrentTotalSize, Peers: peers}
	torrentMapMutex.Unlock()
}

// IsProgressNotMatchUploaded 判断 Peer 报告进度是否与已上传量不匹配.
func IsProgressNotMatchUploaded(torrentTotalSize int64, clientProgress float64, clientUploaded int64) bool {
	if config.BanByProgressUploaded && torrentTotalSize > 0 && clientProgress >= 0 && clientUploaded > 0 {
		startUploaded := (float64(torrentTotalSize) * (config.BanByPUStartPercent / 100))
		peerReportDownloaded := (float64(torrentTotalSize) * clientProgress)
		if (clientUploaded/1024/1024) >= int64(config.BanByPUStartMB) && float64(clientUploaded) >= startUploaded && (peerReportDownloaded*config.BanByPUAntiErrorRatio) < float64(clientUploaded) {
			return true
		}
	}
	return false
}

// IsProgressNotMatchUploaded_Relative 判断 Peer 在两个周期之间的相对上传进度是否不匹配.
func IsProgressNotMatchUploaded_Relative(torrentTotalSize int64, peerInfo PeerInfoStruct, lastPeerInfo PeerInfoStruct) int64 {
	var relativeUploaded int64 = 0
	if peerInfo.Uploaded < lastPeerInfo.Uploaded {
		relativeUploaded = peerInfo.Uploaded
	} else {
		relativeUploaded = (peerInfo.Uploaded - lastPeerInfo.Uploaded)
	}

	if torrentTotalSize > 0 && peerInfo.Uploaded > 0 && (float64(relativeUploaded)/1024/1024) > float64(config.BanByRelativePUStartMB) {
		var relativeUploadedPercent float64 = 0
		if peerInfo.Uploaded > 0 {
			if peerInfo.Uploaded < lastPeerInfo.Uploaded {
				relativeUploadedPercent = 1
			} else {
				relativeUploadedPercent = (1 - (float64(lastPeerInfo.Uploaded) / float64(peerInfo.Uploaded)))
			}
		}
		if relativeUploadedPercent > (config.BanByRelativePUStartPercent / 100) {
			var peerReportProgress float64 = 0
			if peerInfo.Progress > 0 {
				if peerInfo.Progress < lastPeerInfo.Progress {
					peerReportProgress = 1
				} else {
					peerReportProgress = (1 - (lastPeerInfo.Progress / peerInfo.Progress))
				}
			}
			if relativeUploadedPercent > (peerReportProgress * config.BanByRelativePUAntiErrorRatio) {
				return relativeUploaded
			}
		}
	}
	return 0
}

// CheckAllTorrent 对所有种子和 Peer 进行分析.
func CheckAllTorrent(torrentMap map[string]TorrentInfoStruct, lastTorrentMap map[string]TorrentInfoStruct) (int, int) {
	if ((config.IPUploadedCheck && config.IPUpCheckPerTorrentRatio > 0) || config.BanByRelativeProgressUploaded || config.BTNSubmitHistories) && (currentTimestamp > (lastTorrentCleanTimestamp + int64(config.TorrentMapCleanInterval))) {
		blockCount := 0
		ipBlockCount := 0

		torrentMapMutex.Lock()
		lastTorrentMapMutex.Lock()
		defer torrentMapMutex.Unlock()
		defer lastTorrentMapMutex.Unlock()

		for torrentInfoHash, torrentInfo := range torrentMap {
			for peerIP, peerInfo := range torrentInfo.Peers {
				lastTorrentInfo, exist := lastTorrentMap[torrentInfoHash]
				if exist {
					if lastPeerInfo, exist := lastTorrentInfo.Peers[peerIP]; exist {
						if lastPeerInfo.Uploaded == peerInfo.Uploaded {
							continue
						}
					}
				}

				if IsBlockedPeer(peerIP, -1, true) {
					continue
				}

				if config.IPUploadedCheck && config.IPUpCheckPerTorrentRatio > 0 {
					if float64(peerInfo.Uploaded) > (float64(torrentInfo.Size) * peerInfo.Progress * config.IPUpCheckPerTorrentRatio) {
						Log("CheckAllTorrent_AddBlockPeer (Torrent-Too high uploaded)", "%s (Uploaded: %.2f MB)", true, peerIP, (float64(peerInfo.Uploaded) / 1024 / 1024))
						ipBlockCount++
						AddBlockPeer("CheckAllTorrent", "Torrent-Too high uploaded", peerIP, -1, torrentInfoHash, peerInfo.ID, peerInfo.Client, 0, peerInfo.Uploaded)
						AddBlockCIDR(peerIP, peerInfo.Net)
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
								Log("CheckAllTorrent_AddBlockPeer (Bad-Relative_Progress_Uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, peerIP, port, uploadDuring)
								blockCount++
								AddBlockPeer("CheckAllTorrent", "Bad-Relative_Progress_Uploaded", peerIP, port, torrentInfoHash, peerInfo.ID, peerInfo.Client, 0, peerInfo.Uploaded)
								AddBlockCIDR(peerIP, peerInfo.Net)
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

// CheckTorrent 检查单个种子的状态.
func CheckTorrent(torrent *Torrent) (int, []*Peer) {
	if torrent.Hash == "" {
		return -1, nil
	}

	if config.IgnorePTTorrent && torrent.Tracker != "" {
		if torrent.Tracker == "Private" {
			return -4, nil
		}

		lowerTorrentTracker := strings.ToLower(torrent.Tracker)
		if strings.Contains(lowerTorrentTracker, "?passkey=") || strings.Contains(lowerTorrentTracker, "?authkey=") || strings.Contains(lowerTorrentTracker, "?secure=") {
			return -4, nil
		}

		randomStrMatched, err := randomStrRegexp.MatchString(lowerTorrentTracker)
		if err != nil {
			Log("CheckTorrent_MatchTracker", GetLangText("Error-MatchRegexpErr"), true, err.Error())
		} else if randomStrMatched {
			return -4, nil
		}
	}

	if config.IgnoreNoLeechersTorrent && torrent.LeechCount <= 0 {
		return -2, nil
	}

	if torrent.Peers != nil {
		return 0, torrent.Peers
	}

	peers, err := FetchTorrentPeers(torrent)
	if err != nil || peers == nil {
		return -3, nil
	}

	return 0, peers
}

// ProcessTorrent 处理单个种子的 Peer 分析任务.
func ProcessTorrent(torrent *Torrent, emptyHashCount *int, noLeechersCount *int, badTorrentInfoCount *int, ptTorrentCount *int, blockCount *int, ipBlockCount *int, badPeersCount *int, emptyPeersCount *int) {
	torrent.Hash = strings.ToLower(torrent.Hash)
	torrentStatus, peers := CheckTorrent(torrent)
	if config.Debug_CheckTorrent {
		Log("Debug-CheckTorrent", "%s (Status: %d)", false, torrent.Hash, torrentStatus)
	}

	skipSleep := false

	switch torrentStatus {
	case -1:
		skipSleep = true
		*emptyHashCount++
	case -2:
		skipSleep = true
		*noLeechersCount++
	case -3:
		*badTorrentInfoCount++
	case -4:
		skipSleep = true
		*ptTorrentCount++
	case 0:
		for _, peer := range peers {
			ProcessPeer(peer, torrent.Hash, torrent.TotalSize, blockCount, ipBlockCount, badPeersCount, emptyPeersCount)
		}
	}

	if !skipSleep && config.SleepTime != 0 {
		time.Sleep(time.Duration(config.SleepTime) * time.Millisecond)
	}
}
