package main

import (
	"net"
	"time"
	"strings"
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
}

var torrentMap = make(map[string]TorrentInfoStruct)
var lastTorrentMap = make(map[string]TorrentInfoStruct)
var lastTorrentCleanTimestamp int64 = 0

func AddTorrentInfo(torrentInfoHash string, torrentTotalSize int64, cidr *net.IPNet, peerIP string, peerPort int, peerProgress float64, peerUploaded int64) {
	if !((config.IPUploadedCheck && config.IPUpCheckPerTorrentRatio > 0) || config.BanByRelativeProgressUploaded || config.SyncServerURL != "") {
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

	peers[peerIP] = PeerInfoStruct { Net: cidr, Port: peerPortMap, Progress: peerProgress, Uploaded: peerUploaded }
	torrentMap[torrentInfoHash] = TorrentInfoStruct { Size: torrentTotalSize, Peers: peers }
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
func CheckAllTorrent(torrentMap map[string]TorrentInfoStruct, lastTorrentMap map[string]TorrentInfoStruct) (int, int) {
	if ((config.IPUploadedCheck && config.IPUpCheckPerTorrentRatio > 0) || config.BanByRelativeProgressUploaded) && len(lastTorrentMap) > 0 && currentTimestamp > (lastTorrentCleanTimestamp + int64(config.TorrentMapCleanInterval)) {
		blockCount := 0
		ipBlockCount := 0

		for torrentInfoHash, torrentInfo := range torrentMap {
			for peerIP, peerInfo := range torrentInfo.Peers {
				if IsBlockedPeer(peerIP, -1, true) {
					continue
				}

				if config.IPUploadedCheck && config.IPUpCheckPerTorrentRatio > 0 {
					if float64(peerInfo.Uploaded) > (float64(torrentInfo.Size) * peerInfo.Progress * config.IPUpCheckPerTorrentRatio) {
						Log("CheckAllTorrent_AddBlockPeer (Torrent-Too high uploaded)", "%s (Uploaded: %.2f MB)", true, peerIP, (float64(peerInfo.Uploaded) / 1024 / 1024))
						ipBlockCount++
						AddBlockPeer("Torrent-Too high uploaded", peerIP, -1, torrentInfoHash)
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
								AddBlockPeer("Bad-Relative_Progress_Uploaded", peerIP, port, torrentInfoHash)
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
func CheckTorrent(torrentInfoHash string, torrentTracker string, torrentLeecherCount int64, torrentPeers interface{}) (int, interface{}) {
	if torrentInfoHash == "" {
		return -1, nil
	}

	if config.IgnorePTTorrent && torrentTracker != "" {
		if torrentTracker == "Private" {
			return -4, nil
		}

		lowerTorrentTracker := strings.ToLower(torrentTracker)
		if strings.Contains(lowerTorrentTracker, "?passkey=") || strings.Contains(lowerTorrentTracker, "?authkey=") || strings.Contains(lowerTorrentTracker, "?secure=") || randomStrRegexp.MatchString(lowerTorrentTracker) {
			return -4, nil
		}
	}

	if torrentLeecherCount <= 0 {
		return -2, nil
	}

	if torrentPeers != nil {
		return 0, torrentPeers
	}

	torrentPeers = FetchTorrentPeers(torrentInfoHash)
	if torrentPeers == nil {
		return -3, nil
	}

	return 0, torrentPeers
}
func ProcessTorrent(torrentInfoHash string, torrentTracker string, torrentLeecherCount int64, torrentTotalSize int64, torrentPeers interface{}, emptyHashCount *int, noLeechersCount *int, badTorrentInfoCount *int, ptTorrentCount *int, blockCount *int, ipBlockCount *int, badPeersCount *int, emptyPeersCount *int) {
	torrentInfoHash = strings.ToLower(torrentInfoHash)
	torrentStatus, torrentPeersStruct := CheckTorrent(torrentInfoHash, torrentTracker, torrentLeecherCount, torrentPeers)
	if config.Debug_CheckTorrent {
		Log("Debug-CheckTorrent", "%s (Status: %d)", false, torrentInfoHash, torrentStatus)
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
			switch currentClientType {
				case "qBittorrent":
					torrentPeers := torrentPeersStruct.(*qB_TorrentPeersStruct).Peers
					for _, peer := range torrentPeers {
						ProcessPeer(peer.IP, peer.Port, peer.PeerID, peer.Client, peer.DlSpeed, peer.UpSpeed, peer.Progress, peer.Downloaded, peer.Uploaded, torrentInfoHash, torrentTotalSize, blockCount, ipBlockCount, badPeersCount, emptyPeersCount)
					}
				case "Transmission":
					torrentPeers := torrentPeersStruct.([]Tr_PeerStruct)
					for _, peer := range torrentPeers {
						// Transmission 目前似乎并不提供 Peer 的 PeerID 及 Downloaded/Uploaded, 因此使用无效值取代.
						ProcessPeer(peer.IP, peer.Port, "", peer.Client, peer.DlSpeed, peer.UpSpeed, peer.Progress, -1, -1, torrentInfoHash, torrentTotalSize, blockCount, ipBlockCount, badPeersCount, emptyPeersCount)
					}
				case "BitComet":
					torrentPeers := torrentPeersStruct.(*[]BC_PeerStruct)
					for _, peer := range *torrentPeers {
						// BitComet 目前不为其支持 PeerID， 因此使用无效值取代.
						ProcessPeer(peer.IP, peer.Port, "", peer.Client, peer.DlSpeed, peer.UpSpeed, peer.Progress, peer.Downloaded, peer.Uploaded, torrentInfoHash, torrentTotalSize, blockCount, ipBlockCount, badPeersCount, emptyPeersCount)
					}
			}
	}

	if !skipSleep && config.SleepTime != 0 {
		time.Sleep(time.Duration(config.SleepTime) * time.Millisecond)
	}
}
