package stats

import "net"

type IPInfoStruct struct {
	TorrentPeers            map[string]map[int]PeerTrafficCounter `json:"-"`
	TorrentObservedUploaded map[string]int64                      `json:"-"`
	LastSeen                int64                                 `json:"-"`
	TorrentLastSeen         map[string]int64                      `json:"-"`
	Net                     *net.IPNet
	Port                    map[int]bool
	TorrentDownloaded       map[string]int64
	TorrentUploaded         map[string]int64
}

func (s *Store) AddIPInfo(cidr *net.IPNet, peerIP string, peerPort int, torrentInfoHash string, peerDownloaded int64, peerUploaded int64, peerIDs ...string) {
	if !(s.settings().MaxIPPortCount > 0 || (s.settings().IPUploadedCheck && s.settings().IPUpCheckIncrementMB > 0)) {
		return
	}

	var torrentPeers map[string]map[int]PeerTrafficCounter
	var observedUploaded map[string]int64
	var torrentLastSeen map[string]int64
	var clientPortMap map[int]bool
	var clientTorrentDownloadedMap map[string]int64
	var clientTorrentUploadedMap map[string]int64
	s.state.IPMutex.Lock()
	if info, exist := s.state.IPMap[peerIP]; !exist {
		clientPortMap = make(map[int]bool)
		clientTorrentDownloadedMap = make(map[string]int64)
		clientTorrentUploadedMap = make(map[string]int64)
	} else {
		PruneIPPorts(&info, s.now(), EffectiveHistoryRetention(s.settings()))
		torrentPeers = info.TorrentPeers
		observedUploaded = info.TorrentObservedUploaded
		torrentLastSeen = info.TorrentLastSeen
		clientPortMap = info.Port
		clientTorrentDownloadedMap = info.TorrentDownloaded
		clientTorrentUploadedMap = info.TorrentUploaded
		if clientTorrentDownloadedMap == nil {
			clientTorrentDownloadedMap = make(map[string]int64)
		}
	}
	if torrentLastSeen == nil {
		torrentLastSeen = make(map[string]int64)
	}
	if torrentPeers == nil {
		torrentPeers = make(map[string]map[int]PeerTrafficCounter)
	}
	if torrentPeers[torrentInfoHash] == nil {
		torrentPeers[torrentInfoHash] = make(map[int]PeerTrafficCounter)
	}
	if observedUploaded == nil {
		observedUploaded = make(map[string]int64)
	}
	previous, seen := torrentPeers[torrentInfoHash][peerPort]
	if !seen {
		previous.Downloaded, previous.Uploaded = -1, -1
	}
	previous, _ = AlignPeerTrafficCounter(previous, peerIDs...)
	uploadedDelta := CounterDelta(peerUploaded, previous.Uploaded)
	counter := previous
	if peerDownloaded >= 0 {
		counter.Downloaded = peerDownloaded
	}
	if peerUploaded >= 0 {
		counter.Uploaded = peerUploaded
	}
	counter.LastSeen = s.now()
	torrentPeers[torrentInfoHash][peerPort] = counter
	if seen && previous.Uploaded >= 0 && peerUploaded >= 0 {
		observedUploaded[torrentInfoHash] += uploadedDelta
	} else if _, exists := observedUploaded[torrentInfoHash]; !exists {
		observedUploaded[torrentInfoHash] = 0
	}
	torrentLastSeen[torrentInfoHash] = s.now()
	clientPortMap[peerPort] = true
	clientTorrentDownloadedMap[torrentInfoHash] = AccumulateCounter(clientTorrentDownloadedMap[torrentInfoHash], peerDownloaded, previous.Downloaded)
	clientTorrentUploadedMap[torrentInfoHash] = AccumulateCounter(clientTorrentUploadedMap[torrentInfoHash], peerUploaded, previous.Uploaded)

	s.state.IPMap[peerIP] = IPInfoStruct{TorrentPeers: torrentPeers, TorrentObservedUploaded: observedUploaded, LastSeen: s.now(), TorrentLastSeen: torrentLastSeen, Net: cidr, Port: clientPortMap, TorrentDownloaded: clientTorrentDownloadedMap, TorrentUploaded: clientTorrentUploadedMap}
	s.state.IPMutex.Unlock()
}
func (s *Store) IsIPTooHighUploaded(ipInfo IPInfoStruct, lastIPInfo IPInfoStruct) int64 {
	totalUploaded := IPUploadedDelta(ipInfo, lastIPInfo)

	if s.settings().IPUpCheckIncrementMB > 0 {
		var totalUploadedMB int64 = (totalUploaded / 1024 / 1024)
		if totalUploadedMB > int64(s.settings().IPUpCheckIncrementMB) {
			return totalUploadedMB
		}
	}

	return 0
}

// IPUploadedDelta 不将各连接的首个样本计入观测区间的增量.
func IPUploadedDelta(info, previous IPInfoStruct) int64 {
	currentCounters, previousCounters := info.TorrentUploaded, previous.TorrentUploaded
	if info.TorrentObservedUploaded != nil {
		currentCounters, previousCounters = info.TorrentObservedUploaded, previous.TorrentObservedUploaded
	}
	var uploaded int64
	for hash, current := range currentCounters {
		if last, exists := previousCounters[hash]; exists || info.TorrentObservedUploaded != nil {
			uploaded += CounterDelta(current, last)
		}
	}
	return uploaded
}

func (s *Store) CheckAllIP() int {
	s.state.IPMutex.Lock()
	s.state.LastIPMutex.Lock()
	defer s.state.IPMutex.Unlock()
	defer s.state.LastIPMutex.Unlock()
	// 首轮同样检查端口数量; 每个连接的首次原始计数只用于建立基线.
	if (s.settings().MaxIPPortCount > 0 || (s.settings().IPUploadedCheck && s.settings().IPUpCheckIncrementMB > 0)) && s.now() > (s.state.LastIPClean+int64(s.settings().IPUpCheckInterval)) {
		ipBlockCount := 0

		retention := EffectiveHistoryRetention(s.settings())
		for ip, info := range s.state.IPMap {
			PruneIPPorts(&info, s.now(), retention)
			s.state.IPMap[ip] = info
		}

		// 按真实 IP 保存连接历史; 仅在判断上传量时按网段聚合.
		uploadedByNetwork := make(map[string]int64)
		// 观测窗口超过连接历史的保留期限时, 必须重新建立基线.
		if s.settings().IPUploadedCheck && (retention == 0 || s.state.LastIPClean == 0 || s.now()-s.state.LastIPClean <= retention) {
			for ip, info := range s.state.IPMap {
				if info.LastSeen > 0 && info.LastSeen <= s.state.LastIPClean {
					continue
				}
				if len(info.Port) > 0 && !s.isBlocked(ip, -1) {
					uploadedByNetwork[s.IPUploadGroup(ip)] += IPUploadedDelta(info, s.state.LastIPMap[ip])
				}
			}
		}

	ipMapLoop:
		for ip, ipInfo := range s.state.IPMap {
			if ipInfo.LastSeen > 0 && ipInfo.LastSeen <= s.state.LastIPClean {
				continue
			}
			if s.isBlocked(ip, -1) || len(ipInfo.Port) <= 0 {
				continue
			}

			for port := range ipInfo.Port {
				if s.isBlocked(ip, port) {
					continue ipMapLoop
				}
			}

			if s.settings().MaxIPPortCount > 0 {
				if len(ipInfo.Port) > int(s.settings().MaxIPPortCount) {
					s.log("CheckAllIP_AddBlockPeer (Too many ports)", "%s:%d", true, ip, -1)
					ipBlockCount++
					s.BlockIPFromStatistics(ip, "Too many ports", ipInfo)
					continue
				}
			}

			if s.settings().IPUpCheckIncrementMB > 0 {
				if uploadDuring := uploadedByNetwork[s.IPUploadGroup(ip)] / 1024 / 1024; uploadDuring > int64(s.settings().IPUpCheckIncrementMB) {
					s.log("CheckAllIP_AddBlockPeer (Global-Too high uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, ip, -1, uploadDuring)
					ipBlockCount++

					ipInfo.Net = s.network(ip)
					s.BlockIPFromStatistics(ip, "Global-Too high uploaded", ipInfo)
				}
			}
		}

		s.state.LastIPClean = s.now()
		DeepCopyIPMap(s.state.IPMap, s.state.LastIPMap)

		return ipBlockCount
	}

	return 0
}

func (s *Store) IPUploadGroup(ip string) string {
	if cidr := s.network(ip); cidr != nil {
		return cidr.String()
	}
	return ip
}

// BlockIPFromStatistics 保持初始总量与对应连接基线同步.
func (s *Store) BlockIPFromStatistics(ip, reason string, info IPInfoStruct) {
	var downloaded, uploaded int64
	for _, value := range info.TorrentDownloaded {
		if value > 0 {
			downloaded += value
		}
	}
	for _, value := range info.TorrentUploaded {
		if value > 0 {
			uploaded += value
		}
	}
	s.block(BlockEvent{Module: "CheckAllIP", Reason: reason, IP: ip, Port: -1, Downloaded: downloaded, Uploaded: uploaded, Net: info.Net, Counters: copyCounters(info.TorrentPeers)})
}
