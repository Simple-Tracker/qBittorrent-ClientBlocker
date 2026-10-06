package app

import (
	"strings"
	"time"
)

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
