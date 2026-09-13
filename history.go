package main

import "sort"

var lastHistoryCleanTimestamp int64

type historyEntry struct {
	key, sub string
	seen     int64
}

// 超限时优先淘汰最久未观测的记录；只在超过容量时排序。
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

// 与强制 GC 独立，每分钟清理观测历史，同时删除对应基线，避免回归时误判。
func CleanHistory() {
	now := currentTimestamp
	if now <= 0 || (now >= lastHistoryCleanTimestamp && now-lastHistoryCleanTimestamp < 60) {
		return
	}
	lastHistoryCleanTimestamp = now
	cfg := ConfigSnapshot()
	retention := int64(cfg.HistoryRetention)
	if retention > 0 {
		for _, interval := range []uint32{cfg.Interval, cfg.IPUpCheckInterval, cfg.TorrentMapCleanInterval} {
			if minimum := int64(interval) * 2; retention < minimum {
				retention = minimum
			}
		}
	}
	expired := func(seen int64) bool { return retention > 0 && seen > 0 && now-seen > retention }
	ipMapMutex.Lock()
	lastIPMapMutex.Lock()
	removeIP := func(entry historyEntry) { delete(ipMap, entry.key); delete(lastIPMap, entry.key) }
	removeIPTorrent := func(entry historyEntry) {
		info := ipMap[entry.key]
		delete(info.TorrentLastSeen, entry.sub)
		delete(info.TorrentDownloaded, entry.sub)
		delete(info.TorrentUploaded, entry.sub)
		delete(lastIPMap[entry.key].TorrentUploaded, entry.sub)
	}
	ips := make([]historyEntry, 0, len(ipMap))
	for key, info := range ipMap {
		entry := historyEntry{key: key, seen: info.LastSeen}
		if expired(info.LastSeen) {
			removeIP(entry)
		} else {
			ips = append(ips, entry)
		}
	}
	TrimHistory(ips, cfg.HistoryMaxEntries, removeIP)
	var ipTorrents []historyEntry
	for key, info := range ipMap {
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
	lastIPMapMutex.Unlock()
	ipMapMutex.Unlock()

	torrentMapMutex.Lock()
	lastTorrentMapMutex.Lock()
	removePeer := func(entry historyEntry) {
		delete(torrentMap[entry.key].Peers, entry.sub)
		delete(lastTorrentMap[entry.key].Peers, entry.sub)
	}
	var peers []historyEntry
	for hash, torrent := range torrentMap {
		for ip, peer := range torrent.Peers {
			entry := historyEntry{key: hash, sub: ip, seen: peer.LastSeen}
			if expired(peer.LastSeen) {
				removePeer(entry)
			} else {
				peers = append(peers, entry)
			}
		}
	}
	TrimHistory(peers, cfg.HistoryMaxEntries, removePeer)
	for hash, torrent := range torrentMap {
		if len(torrent.Peers) == 0 {
			delete(torrentMap, hash)
			delete(lastTorrentMap, hash)
		}
	}
	lastTorrentMapMutex.Unlock()
	torrentMapMutex.Unlock()
}
