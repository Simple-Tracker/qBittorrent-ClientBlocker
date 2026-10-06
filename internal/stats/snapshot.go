package stats

import "net"

// 参考: https://stackoverflow.com/questions/51459083/deep-copying-maps-in-golang.
func DeepCopyIPMap(src map[string]IPInfoStruct, dest map[string]IPInfoStruct) {
	if src != nil && dest != nil {
		for k := range dest {
			delete(dest, k)
		}
		for k, v := range src {
			newPortMap := make(map[int]bool)
			for pk, pv := range v.Port {
				newPortMap[pk] = pv
			}
			newTorrentUploadedMap := make(map[string]int64)
			for tk, tv := range v.TorrentUploaded {
				newTorrentUploadedMap[tk] = tv
			}
			observed := make(map[string]int64)
			for hash, uploaded := range v.TorrentObservedUploaded {
				observed[hash] = uploaded
			}
			if v.TorrentObservedUploaded == nil {
				observed = nil
			}
			dest[k] = IPInfoStruct{
				TorrentObservedUploaded: observed,
				LastSeen:                v.LastSeen,
				Net:                     v.Net,
				Port:                    newPortMap,
				TorrentUploaded:         newTorrentUploadedMap,
			}
		}
	}
}
func DeepCopyTorrentMap(src map[string]TorrentInfoStruct, dest map[string]TorrentInfoStruct) {
	if src != nil && dest != nil {
		for k := range dest {
			delete(dest, k)
		}
		for k, v := range src {
			newPeers := make(map[string]PeerInfoStruct)
			for pk, pv := range v.Peers {
				newPeers[pk] = CopyPeerInfo(pv)
			}
			dest[k] = TorrentInfoStruct{
				Size:  v.Size,
				Peers: newPeers,
			}
		}
	}
}
func CopyPeerInfo(info PeerInfoStruct) PeerInfoStruct {
	info.Net = copyNetwork(info.Net)
	ports := make(map[int]bool, len(info.Port))
	for port, enabled := range info.Port {
		ports[port] = enabled
	}
	info.Port = ports
	if info.Connections != nil {
		connections := make(map[int]PeerInfoStruct, len(info.Connections))
		for port, peer := range info.Connections {
			connections[port] = CopyPeerInfo(peer)
		}
		info.Connections = connections
	}
	return info
}

func copyCounters(src map[string]map[int]PeerTrafficCounter) map[string]map[int]PeerTrafficCounter {
	if src == nil {
		return nil
	}
	result := make(map[string]map[int]PeerTrafficCounter, len(src))
	for hash, peers := range src {
		copied := make(map[int]PeerTrafficCounter, len(peers))
		for port, counter := range peers {
			copied[port] = counter
		}
		result[hash] = copied
	}
	return result
}

func copyStringCounters(src map[string]int64) map[string]int64 {
	if src == nil {
		return nil
	}
	result := make(map[string]int64, len(src))
	for key, value := range src {
		result[key] = value
	}
	return result
}

func copyIPInfo(info IPInfoStruct) IPInfoStruct {
	info.TorrentPeers = copyCounters(info.TorrentPeers)
	info.TorrentObservedUploaded = copyStringCounters(info.TorrentObservedUploaded)
	info.TorrentLastSeen = copyStringCounters(info.TorrentLastSeen)
	info.TorrentDownloaded = copyStringCounters(info.TorrentDownloaded)
	info.TorrentUploaded = copyStringCounters(info.TorrentUploaded)
	ports := make(map[int]bool, len(info.Port))
	for port, enabled := range info.Port {
		ports[port] = enabled
	}
	info.Port = ports
	info.Net = copyNetwork(info.Net)
	return info
}

func copyNetwork(network *net.IPNet) *net.IPNet {
	if network == nil {
		return nil
	}
	return &net.IPNet{IP: append(net.IP(nil), network.IP...), Mask: append(net.IPMask(nil), network.Mask...)}
}

// IPSnapshot 返回当前 IP 历史记录的独立副本.
func (s *Store) IPSnapshot() map[string]IPInfoStruct {
	s.state.IPMutex.RLock()
	defer s.state.IPMutex.RUnlock()
	result := make(map[string]IPInfoStruct, len(s.state.IPMap))
	for ip, info := range s.state.IPMap {
		result[ip] = copyIPInfo(info)
	}
	return result
}

// TorrentSnapshot 返回当前种子历史记录的独立副本.
func (s *Store) TorrentSnapshot() map[string]TorrentInfoStruct {
	s.state.TorrentMutex.RLock()
	defer s.state.TorrentMutex.RUnlock()
	result := make(map[string]TorrentInfoStruct, len(s.state.TorrentMap))
	DeepCopyTorrentMap(s.state.TorrentMap, result)
	return result
}

// Snapshot 同时获取样本和检测基线, 供测试构造状态使用.
func (s *Store) Snapshot() *State {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()
	s.state.IPMutex.RLock()
	defer s.state.IPMutex.RUnlock()
	s.state.LastIPMutex.RLock()
	defer s.state.LastIPMutex.RUnlock()
	s.state.TorrentMutex.RLock()
	defer s.state.TorrentMutex.RUnlock()
	s.state.LastTorrentMutex.RLock()
	defer s.state.LastTorrentMutex.RUnlock()
	result := &State{IPMap: make(map[string]IPInfoStruct), LastIPMap: make(map[string]IPInfoStruct), TorrentMap: make(map[string]TorrentInfoStruct), LastTorrentMap: make(map[string]TorrentInfoStruct), LastIPClean: s.state.LastIPClean, LastTorrentClean: s.state.LastTorrentClean, LastHistoryClean: s.state.LastHistoryClean}
	for ip, info := range s.state.IPMap {
		result.IPMap[ip] = copyIPInfo(info)
	}
	for ip, info := range s.state.LastIPMap {
		result.LastIPMap[ip] = copyIPInfo(info)
	}
	DeepCopyTorrentMap(s.state.TorrentMap, result.TorrentMap)
	DeepCopyTorrentMap(s.state.LastTorrentMap, result.LastTorrentMap)
	return result
}
