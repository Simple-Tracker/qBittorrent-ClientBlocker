package stats

import "net"

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

// AddTorrentInfo 添加种子信息, 以便后续进行上传进度比分析.
func (s *Store) AddTorrentInfo(torrentInfoHash string, torrentTotalSize int64, cidr *net.IPNet, peerIP string, peerPort int, peerProgress float64, peerDownloaded int64, peerUploaded int64, peerID string, peerClient string) {
	if !((s.settings().IPUploadedCheck && s.settings().IPUpCheckPerTorrentRatio > 0) || s.settings().BanByRelativeProgressUploaded || s.settings().SyncServerURL != "" || s.settings().BTNSubmitPeers || s.settings().BTNSubmitHistories) {
		return
	}

	var peers map[string]PeerInfoStruct
	var peerPortMap map[int]bool
	s.state.TorrentMutex.Lock()
	if torrentInfo, exist := s.state.TorrentMap[torrentInfoHash]; !exist {
		peers = make(map[string]PeerInfoStruct)
		peerPortMap = make(map[int]bool)
	} else {
		peers = torrentInfo.Peers
		if peerInfo, exist := peers[peerIP]; !exist {
			peerPortMap = make(map[int]bool)
		} else {
			s.state.LastTorrentMutex.Lock()
			previous := s.state.LastTorrentMap[torrentInfoHash].Peers[peerIP]
			PruneTorrentPorts(&peerInfo, &previous, s.now(), EffectiveHistoryRetention(s.settings()))
			s.state.LastTorrentMutex.Unlock()
			peers[peerIP] = peerInfo
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
		firstSeen = s.now()
	}
	if info.FirstSeen == 0 {
		info.FirstSeen = info.LastSeen
	}
	if info.FirstSeen == 0 {
		info.FirstSeen = s.now()
	}
	counter := previous.Counters
	if !seen {
		counter.Downloaded, counter.Uploaded = -1, -1
	}
	if counter.PeerID == "" {
		counter.PeerID = previous.ID
	}
	counter, identityChanged := AlignPeerTrafficCounter(counter, peerID)
	previousDownloaded, previousUploaded := counter.Downloaded, counter.Uploaded
	session := previous.Session
	if (peerDownloaded >= 0 && peerDownloaded < previousDownloaded) || (peerUploaded >= 0 && peerUploaded < previousUploaded) || identityChanged {
		session++
	}
	if peerDownloaded >= 0 {
		counter.Downloaded = peerDownloaded
	}
	if peerUploaded >= 0 {
		counter.Uploaded = peerUploaded
	}
	counter.LastSeen = s.now()
	info.Connections[peerPort] = PeerInfoStruct{
		Counters:  counter,
		FirstSeen: firstSeen, LastSeen: s.now(), Net: cidr, Port: map[int]bool{peerPort: true},
		Progress: peerProgress, Downloaded: AccumulateCounter(previous.Downloaded, peerDownloaded, previousDownloaded), Uploaded: AccumulateCounter(previous.Uploaded, peerUploaded, previousUploaded),
		RawDownloaded: peerDownloaded, RawUploaded: peerUploaded, ID: peerID, Client: peerClient, Session: session,
	}
	peers[peerIP] = PeerInfoStruct{Connections: info.Connections, FirstSeen: info.FirstSeen, LastSeen: s.now(), Net: cidr, Port: peerPortMap, Progress: peerProgress, Downloaded: AccumulateCounter(info.Downloaded, peerDownloaded, previousDownloaded), Uploaded: AccumulateCounter(info.Uploaded, peerUploaded, previousUploaded), ID: peerID, Client: peerClient}
	s.state.TorrentMap[torrentInfoHash] = TorrentInfoStruct{Size: torrentTotalSize, Peers: peers}
	s.state.TorrentMutex.Unlock()
}

// PeerConnections 将流量和进度与实际端点对应保存.
func PeerConnections(info PeerInfoStruct) map[int]PeerInfoStruct {
	if info.Connections != nil {
		return info.Connections
	}
	return map[int]PeerInfoStruct{0: info}
}

// IsProgressNotMatchUploaded 判断 Peer 报告进度是否与已上传量不匹配.
func (s *Store) IsProgressNotMatchUploaded(torrentTotalSize int64, clientProgress float64, clientUploaded int64) bool {
	if s.settings().BanByProgressUploaded && torrentTotalSize > 0 && clientProgress >= 0 && clientUploaded > 0 {
		startUploaded := (float64(torrentTotalSize) * (s.settings().BanByPUStartPercent / 100))
		peerReportDownloaded := (float64(torrentTotalSize) * clientProgress)
		if (clientUploaded/1024/1024) >= int64(s.settings().BanByPUStartMB) && float64(clientUploaded) >= startUploaded && (peerReportDownloaded*s.settings().BanByPUAntiErrorRatio) < float64(clientUploaded) {
			return true
		}
	}
	return false
}

// IsProgressNotMatchUploaded_Relative 判断 Peer 在两个周期之间的相对上传进度是否不匹配.
func (s *Store) IsProgressNotMatchUploaded_Relative(torrentTotalSize int64, peerInfo PeerInfoStruct, lastPeerInfo PeerInfoStruct) int64 {
	var relativeUploaded int64 = 0
	if peerInfo.Uploaded < lastPeerInfo.Uploaded {
		relativeUploaded = peerInfo.Uploaded
	} else {
		relativeUploaded = (peerInfo.Uploaded - lastPeerInfo.Uploaded)
	}

	if torrentTotalSize > 0 && peerInfo.Uploaded > 0 && (float64(relativeUploaded)/1024/1024) > float64(s.settings().BanByRelativePUStartMB) {
		var relativeUploadedPercent float64 = 0
		if peerInfo.Uploaded > 0 {
			if peerInfo.Uploaded < lastPeerInfo.Uploaded {
				relativeUploadedPercent = 1
			} else {
				relativeUploadedPercent = (1 - (float64(lastPeerInfo.Uploaded) / float64(peerInfo.Uploaded)))
			}
		}
		if relativeUploadedPercent > (s.settings().BanByRelativePUStartPercent / 100) {
			var peerReportProgress float64 = 0
			if peerInfo.Progress > 0 {
				if peerInfo.Progress < lastPeerInfo.Progress {
					peerReportProgress = 1
				} else {
					peerReportProgress = (1 - (lastPeerInfo.Progress / peerInfo.Progress))
				}
			}
			if relativeUploadedPercent > (peerReportProgress * s.settings().BanByRelativePUAntiErrorRatio) {
				return relativeUploaded
			}
		}
	}
	return 0
}

// CheckAllTorrent 对所有种子和 Peer 进行分析.
func (s *Store) CheckAllTorrent() (int, int) {
	s.state.TorrentMutex.Lock()
	s.state.LastTorrentMutex.Lock()
	defer s.state.TorrentMutex.Unlock()
	defer s.state.LastTorrentMutex.Unlock()
	if ((s.settings().IPUploadedCheck && s.settings().IPUpCheckPerTorrentRatio > 0) || s.settings().BanByRelativeProgressUploaded || s.settings().BTNSubmitHistories) && (s.now() > (s.state.LastTorrentClean + int64(s.settings().TorrentMapCleanInterval))) {
		blockCount := 0
		ipBlockCount := 0

		for torrentInfoHash, torrentInfo := range s.state.TorrentMap {
			for peerIP, info := range torrentInfo.Peers {
				previous := s.state.LastTorrentMap[torrentInfoHash].Peers[peerIP]
				PruneTorrentPorts(&info, &previous, s.now(), EffectiveHistoryRetention(s.settings()))
				torrentInfo.Peers[peerIP] = info
				for port, peerInfo := range PeerConnections(info) {
					lastTorrentInfo := s.state.LastTorrentMap[torrentInfoHash]
					lastInfo, hasPeer := lastTorrentInfo.Peers[peerIP]
					lastPeerInfo, hasLast := PeerConnections(lastInfo)[port]
					hasLast = hasLast && hasPeer
					if hasLast {
						if lastPeerInfo.Uploaded == peerInfo.Uploaded {
							continue
						}
					}

					if s.isBlocked(peerIP, -1) {
						continue
					}

					uploaded := peerInfo.Uploaded
					if info.Connections != nil {
						uploaded = peerInfo.RawUploaded
					}
					if s.settings().IPUploadedCheck && s.settings().IPUpCheckPerTorrentRatio > 0 {
						if float64(uploaded) > (float64(torrentInfo.Size) * peerInfo.Progress * s.settings().IPUpCheckPerTorrentRatio) {
							s.log("CheckAllTorrent_AddBlockPeer (Torrent-Too high uploaded)", "%s (Uploaded: %.2f MB)", true, peerIP, (float64(peerInfo.Uploaded) / 1024 / 1024))
							ipBlockCount++
							s.blockTorrentPeer("Torrent-Too high uploaded", peerIP, -1, torrentInfoHash, port, peerInfo, info.Connections != nil)
							continue
						}
					}

					if s.settings().BanByRelativeProgressUploaded {
						if hasLast && peerInfo.Session == lastPeerInfo.Session {
							currentProgress, previousProgress := peerInfo, lastPeerInfo
							if info.Connections != nil {
								if peerInfo.RawUploaded < 0 || lastPeerInfo.RawUploaded < 0 {
									continue
								}
								currentProgress.Uploaded, previousProgress.Uploaded = peerInfo.RawUploaded, lastPeerInfo.RawUploaded
							}
							if uploadDuring := s.IsProgressNotMatchUploaded_Relative(torrentInfo.Size, currentProgress, previousProgress); uploadDuring > 0 {
								for port := range peerInfo.Port {
									if s.isBlocked(peerIP, port) {
										continue
									}
									s.log("CheckAllTorrent_AddBlockPeer (Bad-Relative_Progress_Uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, peerIP, port, uploadDuring)
									blockCount++
									s.blockTorrentPeer("Bad-Relative_Progress_Uploaded", peerIP, port, torrentInfoHash, port, peerInfo, info.Connections != nil)
								}
								continue
							}
						}
					}
				}
			}

		}
		s.state.LastTorrentClean = s.now()
		DeepCopyTorrentMap(s.state.TorrentMap, s.state.LastTorrentMap)

		return blockCount, ipBlockCount
	}

	return 0, 0
}
