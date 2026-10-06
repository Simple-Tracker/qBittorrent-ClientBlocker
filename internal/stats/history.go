package stats

import "sort"

type historyEntry struct {
	key, sub string
	seen     int64
}

func EffectiveHistoryRetention(cfg *Settings) int64 {
	retention := int64(cfg.HistoryRetention)
	if retention > 0 {
		for _, interval := range []uint32{cfg.Interval, cfg.IPUpCheckInterval, cfg.TorrentMapCleanInterval} {
			if minimum := int64(interval) * 2; retention < minimum {
				retention = minimum
			}
		}
	}
	return retention
}

// 调用方持有 s.state.IPMutex. 原始样本过期时不能扣减已记录的流量.
func PruneIPPorts(info *IPInfoStruct, now, retention int64) {
	if retention <= 0 || info.TorrentPeers == nil {
		return
	}
	if info.Port == nil {
		info.Port = make(map[int]bool)
	}
	for port := range info.Port {
		delete(info.Port, port)
	}
	for hash, peers := range info.TorrentPeers {
		for port, peer := range peers {
			if peer.LastSeen > 0 && now-peer.LastSeen > retention {
				delete(peers, port)
			} else {
				info.Port[port] = true
			}
		}
		if len(peers) == 0 {
			delete(info.TorrentPeers, hash)
		}
	}
}

// 调用方持有 s.state.TorrentMutex; 传入 previous 时还需持有 s.state.LastTorrentMutex.
func PruneTorrentPorts(info, previous *PeerInfoStruct, now, retention int64) {
	if retention <= 0 || info.Connections == nil {
		return
	}
	if info.Port == nil {
		info.Port = make(map[int]bool)
	}
	for port := range info.Port {
		delete(info.Port, port)
	}
	for port, peer := range info.Connections {
		if peer.LastSeen > 0 && now-peer.LastSeen > retention {
			delete(info.Connections, port)
			if previous != nil {
				delete(previous.Connections, port)
				delete(previous.Port, port)
			}
		} else {
			info.Port[port] = true
		}
	}
}

// 超限时优先淘汰最久未观测的记录; 只在超过容量时排序.
func TrimHistory(entries []historyEntry, limit uint32, remove func(historyEntry)) {
	if limit == 0 || uint64(len(entries)) <= uint64(limit) {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].seen != entries[j].seen {
			return entries[i].seen < entries[j].seen
		}
		if entries[i].key != entries[j].key {
			return entries[i].key < entries[j].key
		}
		return entries[i].sub < entries[j].sub
	})
	for _, entry := range entries[:len(entries)-int(limit)] {
		remove(entry)
	}
}

// 与强制 GC 独立, 每分钟清理观测历史, 同时删除对应基线, 避免回归时误判.
func (s *Store) CleanHistory() {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()
	now := s.now()
	if now <= 0 || (now >= s.state.LastHistoryClean && now-s.state.LastHistoryClean < 60) {
		return
	}
	s.state.LastHistoryClean = now
	cfg := s.settings()
	retention := EffectiveHistoryRetention(cfg)
	expired := func(seen int64) bool { return retention > 0 && seen > 0 && now-seen > retention }
	s.state.IPMutex.Lock()
	s.state.LastIPMutex.Lock()
	removeIP := func(entry historyEntry) { delete(s.state.IPMap, entry.key); delete(s.state.LastIPMap, entry.key) }
	removeIPTorrent := func(entry historyEntry) {
		info := s.state.IPMap[entry.key]
		delete(info.TorrentPeers, entry.sub)
		delete(info.TorrentObservedUploaded, entry.sub)
		delete(s.state.LastIPMap[entry.key].TorrentObservedUploaded, entry.sub)
		delete(info.TorrentLastSeen, entry.sub)
		delete(info.TorrentDownloaded, entry.sub)
		delete(info.TorrentUploaded, entry.sub)
		delete(s.state.LastIPMap[entry.key].TorrentUploaded, entry.sub)
	}
	ips := make([]historyEntry, 0, len(s.state.IPMap))
	for key, info := range s.state.IPMap {
		entry := historyEntry{key: key, seen: info.LastSeen}
		if expired(info.LastSeen) {
			removeIP(entry)
		} else {
			ips = append(ips, entry)
		}
	}
	TrimHistory(ips, cfg.HistoryMaxEntries, removeIP)
	var ipTorrents []historyEntry
	for key, info := range s.state.IPMap {
		for hash, seen := range info.TorrentLastSeen {
			entry := historyEntry{key: key, sub: hash, seen: seen}
			if expired(seen) {
				removeIPTorrent(entry)
			} else {
				ipTorrents = append(ipTorrents, entry)
			}
		}
	}
	TrimHistory(ipTorrents, cfg.HistoryMaxEntries, removeIPTorrent)
	for key, info := range s.state.IPMap {
		PruneIPPorts(&info, now, retention)
		s.state.IPMap[key] = info
	}
	s.state.LastIPMutex.Unlock()
	s.state.IPMutex.Unlock()

	s.state.TorrentMutex.Lock()
	s.state.LastTorrentMutex.Lock()
	removePeer := func(entry historyEntry) {
		delete(s.state.TorrentMap[entry.key].Peers, entry.sub)
		delete(s.state.LastTorrentMap[entry.key].Peers, entry.sub)
	}
	var peers []historyEntry
	for hash, torrent := range s.state.TorrentMap {
		for ip, peer := range torrent.Peers {
			previous := s.state.LastTorrentMap[hash].Peers[ip]
			PruneTorrentPorts(&peer, &previous, now, retention)
			torrent.Peers[ip] = peer
			entry := historyEntry{key: hash, sub: ip, seen: peer.LastSeen}
			if expired(peer.LastSeen) {
				removePeer(entry)
			} else {
				peers = append(peers, entry)
			}
		}
	}
	TrimHistory(peers, cfg.HistoryMaxEntries, removePeer)
	for hash, torrent := range s.state.TorrentMap {
		if len(torrent.Peers) == 0 {
			delete(s.state.TorrentMap, hash)
			delete(s.state.LastTorrentMap, hash)
		}
	}
	s.state.LastTorrentMutex.Unlock()
	s.state.TorrentMutex.Unlock()
}
