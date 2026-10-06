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
	Connections   map[int]PeerInfoStruct `json:"-"`
	Counters      PeerTrafficCounter     `json:"-"`
	Session       uint64                 `json:"-"`
	RawDownloaded int64                  `json:"-"`
	RawUploaded   int64                  `json:"-"`
	FirstSeen     int64                  `json:"-"`
	LastSeen      int64                  `json:"-"`
	Net           *net.IPNet
	Port          map[int]bool
	Progress      float64
	Downloaded    int64
	Uploaded      int64
	ID            string
	Client        string
}

var torrentMap = make(map[string]TorrentInfoStruct)
var lastTorrentMap = make(map[string]TorrentInfoStruct)
var torrentMapMutex sync.RWMutex
var lastTorrentMapMutex sync.RWMutex
var lastTorrentCleanTimestamp int64 = 0

// AddTorrentInfo 添加种子信息, 以便后续进行上传进度比分析.
func AddTorrentInfo(torrentInfoHash string, torrentTotalSize int64, cidr *net.IPNet, peerIP string, peerPort int, peerProgress float64, peerDownloaded int64, peerUploaded int64, peerID string, peerClient string) {
	if !((ConfigSnapshot().IPUploadedCheck && ConfigSnapshot().IPUpCheckPerTorrentRatio > 0) || ConfigSnapshot().BanByRelativeProgressUploaded || ConfigSnapshot().SyncServerURL != "" || ConfigSnapshot().BTNSubmitPeers || ConfigSnapshot().BTNSubmitHistories) {
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

	info := peers[peerIP]
	if info.Connections == nil {
		info.Connections = make(map[int]PeerInfoStruct)
	}
	previous, seen := info.Connections[peerPort]
	firstSeen := previous.FirstSeen
	if firstSeen == 0 {
		firstSeen = previous.LastSeen
	}
	if firstSeen == 0 {
		firstSeen = currentTimestamp
	}
	if info.FirstSeen == 0 {
		info.FirstSeen = info.LastSeen
	}
	if info.FirstSeen == 0 {
		info.FirstSeen = currentTimestamp
	}
	counter := previous.Counters
	if !seen {
		counter.Downloaded, counter.Uploaded = -1, -1
	}
	previousDownloaded, previousUploaded := counter.Downloaded, counter.Uploaded
	session := previous.Session
	if (peerDownloaded >= 0 && peerDownloaded < previousDownloaded) || (peerUploaded >= 0 && peerUploaded < previousUploaded) || (previous.ID != "" && peerID != "" && previous.ID != peerID) {
		session++
	}
	if previous.ID != "" && peerID != "" && previous.ID != peerID {
		previousDownloaded, previousUploaded = 0, 0
	}
	if peerDownloaded >= 0 {
		counter.Downloaded = peerDownloaded
	}
	if peerUploaded >= 0 {
		counter.Uploaded = peerUploaded
	}
	counter.LastSeen = currentTimestamp
	info.Connections[peerPort] = PeerInfoStruct{
		Counters:  counter,
		FirstSeen: firstSeen, LastSeen: currentTimestamp, Net: cidr, Port: map[int]bool{peerPort: true},
		Progress: peerProgress, Downloaded: AccumulateCounter(previous.Downloaded, peerDownloaded, previousDownloaded), Uploaded: AccumulateCounter(previous.Uploaded, peerUploaded, previousUploaded),
		RawDownloaded: peerDownloaded, RawUploaded: peerUploaded, ID: peerID, Client: peerClient, Session: session,
	}
	peers[peerIP] = PeerInfoStruct{Connections: info.Connections, FirstSeen: info.FirstSeen, LastSeen: currentTimestamp, Net: cidr, Port: peerPortMap, Progress: peerProgress, Downloaded: AccumulateCounter(info.Downloaded, peerDownloaded, previousDownloaded), Uploaded: AccumulateCounter(info.Uploaded, peerUploaded, previousUploaded), ID: peerID, Client: peerClient}
	torrentMap[torrentInfoHash] = TorrentInfoStruct{Size: torrentTotalSize, Peers: peers}
	torrentMapMutex.Unlock()
}

// PeerConnections keeps traffic and progress paired with the actual endpoint.
func PeerConnections(info PeerInfoStruct) map[int]PeerInfoStruct {
	if info.Connections != nil {
		return info.Connections
	}
	return map[int]PeerInfoStruct{0: info}
}

// IsProgressNotMatchUploaded 判断 Peer 报告进度是否与已上传量不匹配.
func IsProgressNotMatchUploaded(torrentTotalSize int64, clientProgress float64, clientUploaded int64) bool {
	if ConfigSnapshot().BanByProgressUploaded && torrentTotalSize > 0 && clientProgress >= 0 && clientUploaded > 0 {
		startUploaded := (float64(torrentTotalSize) * (ConfigSnapshot().BanByPUStartPercent / 100))
		peerReportDownloaded := (float64(torrentTotalSize) * clientProgress)
		if (clientUploaded/1024/1024) >= int64(ConfigSnapshot().BanByPUStartMB) && float64(clientUploaded) >= startUploaded && (peerReportDownloaded*ConfigSnapshot().BanByPUAntiErrorRatio) < float64(clientUploaded) {
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

	if torrentTotalSize > 0 && peerInfo.Uploaded > 0 && (float64(relativeUploaded)/1024/1024) > float64(ConfigSnapshot().BanByRelativePUStartMB) {
		var relativeUploadedPercent float64 = 0
		if peerInfo.Uploaded > 0 {
			if peerInfo.Uploaded < lastPeerInfo.Uploaded {
				relativeUploadedPercent = 1
			} else {
				relativeUploadedPercent = (1 - (float64(lastPeerInfo.Uploaded) / float64(peerInfo.Uploaded)))
			}
		}
		if relativeUploadedPercent > (ConfigSnapshot().BanByRelativePUStartPercent / 100) {
			var peerReportProgress float64 = 0
			if peerInfo.Progress > 0 {
				if peerInfo.Progress < lastPeerInfo.Progress {
					peerReportProgress = 1
				} else {
					peerReportProgress = (1 - (lastPeerInfo.Progress / peerInfo.Progress))
				}
			}
			if relativeUploadedPercent > (peerReportProgress * ConfigSnapshot().BanByRelativePUAntiErrorRatio) {
				return relativeUploaded
			}
		}
	}
	return 0
}

// CheckAllTorrent 对所有种子和 Peer 进行分析.
func CheckAllTorrent(torrentMap map[string]TorrentInfoStruct, lastTorrentMap map[string]TorrentInfoStruct) (int, int) {
	if ((ConfigSnapshot().IPUploadedCheck && ConfigSnapshot().IPUpCheckPerTorrentRatio > 0) || ConfigSnapshot().BanByRelativeProgressUploaded || ConfigSnapshot().BTNSubmitHistories) && (currentTimestamp > (lastTorrentCleanTimestamp + int64(ConfigSnapshot().TorrentMapCleanInterval))) {
		blockCount := 0
		ipBlockCount := 0

		torrentMapMutex.Lock()
		lastTorrentMapMutex.Lock()
		defer torrentMapMutex.Unlock()
		defer lastTorrentMapMutex.Unlock()

		for torrentInfoHash, torrentInfo := range torrentMap {
			for peerIP, info := range torrentInfo.Peers {
				for port, peerInfo := range PeerConnections(info) {
					lastTorrentInfo := lastTorrentMap[torrentInfoHash]
					lastInfo, hasPeer := lastTorrentInfo.Peers[peerIP]
					lastPeerInfo, hasLast := PeerConnections(lastInfo)[port]
					hasLast = hasLast && hasPeer
					if hasLast {
						if lastPeerInfo.Uploaded == peerInfo.Uploaded {
							continue
						}
					}

					if IsBlockedPeer(peerIP, -1, false) {
						continue
					}

					uploaded := peerInfo.Uploaded
					if info.Connections != nil {
						uploaded = peerInfo.RawUploaded
					}
					if ConfigSnapshot().IPUploadedCheck && ConfigSnapshot().IPUpCheckPerTorrentRatio > 0 {
						if float64(uploaded) > (float64(torrentInfo.Size) * peerInfo.Progress * ConfigSnapshot().IPUpCheckPerTorrentRatio) {
							Log("CheckAllTorrent_AddBlockPeer (Torrent-Too high uploaded)", "%s (Uploaded: %.2f MB)", true, peerIP, (float64(peerInfo.Uploaded) / 1024 / 1024))
							ipBlockCount++
							AddBlockPeer("CheckAllTorrent", "Torrent-Too high uploaded", peerIP, -1, torrentInfoHash, peerInfo.ID, peerInfo.Client, peerInfo.Downloaded, peerInfo.Uploaded)
							if info.Connections != nil {
								SeedBlockedPeerCounters(peerIP, map[string]map[int]PeerTrafficCounter{torrentInfoHash: {port: peerInfo.Counters}})
							}
							AddBlockCIDR(peerIP, peerInfo.Net)
							continue
						}
					}

					if ConfigSnapshot().BanByRelativeProgressUploaded {
						if hasLast && peerInfo.Session == lastPeerInfo.Session {
							currentProgress, previousProgress := peerInfo, lastPeerInfo
							if info.Connections != nil {
								if peerInfo.RawUploaded < 0 || lastPeerInfo.RawUploaded < 0 {
									continue
								}
								currentProgress.Uploaded, previousProgress.Uploaded = peerInfo.RawUploaded, lastPeerInfo.RawUploaded
							}
							if uploadDuring := IsProgressNotMatchUploaded_Relative(torrentInfo.Size, currentProgress, previousProgress); uploadDuring > 0 {
								for port := range peerInfo.Port {
									if IsBlockedPeer(peerIP, port, false) {
										continue
									}
									Log("CheckAllTorrent_AddBlockPeer (Bad-Relative_Progress_Uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, peerIP, port, uploadDuring)
									blockCount++
									AddBlockPeer("CheckAllTorrent", "Bad-Relative_Progress_Uploaded", peerIP, port, torrentInfoHash, peerInfo.ID, peerInfo.Client, peerInfo.Downloaded, peerInfo.Uploaded)
									if info.Connections != nil {
										SeedBlockedPeerCounters(peerIP, map[string]map[int]PeerTrafficCounter{torrentInfoHash: {port: peerInfo.Counters}})
									}
									AddBlockCIDR(peerIP, peerInfo.Net)
								}
								continue
							}
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

	if ConfigSnapshot().IgnorePTTorrent && torrent.Tracker != "" {
		if torrent.Tracker == "Private" {
			return -4, nil
		}

		lowerTorrentTracker := strings.ToLower(torrent.Tracker)
		if strings.Contains(lowerTorrentTracker, "?passkey=") || strings.Contains(lowerTorrentTracker, "?authkey=") || strings.Contains(lowerTorrentTracker, "?secure=") {
			return -4, nil
		}

		randomStrMatched, err := randomStrRegexp.MatchString(lowerTorrentTracker)
		if err != nil {
			LogError("CheckTorrent_MatchTracker", GetLangText("Error-MatchRegexpErr"), true, err.Error())
		} else if randomStrMatched {
			return -4, nil
		}
	}

	if ConfigSnapshot().IgnoreNoLeechersTorrent && torrent.LeechCount <= 0 {
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
	if ConfigSnapshot().Debug_CheckTorrent {
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

	if !skipSleep && ConfigSnapshot().SleepTime != 0 {
		WaitRequestDelay(time.Duration(ConfigSnapshot().SleepTime) * time.Millisecond)
	}
}
