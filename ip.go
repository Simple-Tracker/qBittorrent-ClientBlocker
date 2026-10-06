package main

import (
	"net"
	"sync"
)

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

var ipMap = make(map[string]IPInfoStruct)
var lastIPMap = make(map[string]IPInfoStruct)
var ipMapMutex sync.RWMutex
var lastIPMapMutex sync.RWMutex
var lastIPCleanTimestamp int64 = 0

func AddIPInfo(cidr *net.IPNet, peerIP string, peerPort int, torrentInfoHash string, peerDownloaded int64, peerUploaded int64) {
	if !(ConfigSnapshot().MaxIPPortCount > 0 || (ConfigSnapshot().IPUploadedCheck && ConfigSnapshot().IPUpCheckIncrementMB > 0)) {
		return
	}

	var torrentPeers map[string]map[int]PeerTrafficCounter
	var observedUploaded map[string]int64
	var torrentLastSeen map[string]int64
	var clientPortMap map[int]bool
	var clientTorrentDownloadedMap map[string]int64
	var clientTorrentUploadedMap map[string]int64
	ipMapMutex.Lock()
	if info, exist := ipMap[peerIP]; !exist {
		clientPortMap = make(map[int]bool)
		clientTorrentDownloadedMap = make(map[string]int64)
		clientTorrentUploadedMap = make(map[string]int64)
	} else {
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
	uploadedDelta := CounterDelta(peerUploaded, previous.Uploaded)
	counter := previous
	if peerDownloaded >= 0 {
		counter.Downloaded = peerDownloaded
	}
	if peerUploaded >= 0 {
		counter.Uploaded = peerUploaded
	}
	counter.LastSeen = currentTimestamp
	torrentPeers[torrentInfoHash][peerPort] = counter
	if seen && previous.Uploaded >= 0 && peerUploaded >= 0 {
		observedUploaded[torrentInfoHash] += uploadedDelta
	} else if _, exists := observedUploaded[torrentInfoHash]; !exists {
		observedUploaded[torrentInfoHash] = 0
	}
	torrentLastSeen[torrentInfoHash] = currentTimestamp
	clientPortMap[peerPort] = true
	clientTorrentDownloadedMap[torrentInfoHash] = AccumulateCounter(clientTorrentDownloadedMap[torrentInfoHash], peerDownloaded, previous.Downloaded)
	clientTorrentUploadedMap[torrentInfoHash] = AccumulateCounter(clientTorrentUploadedMap[torrentInfoHash], peerUploaded, previous.Uploaded)

	ipMap[peerIP] = IPInfoStruct{TorrentPeers: torrentPeers, TorrentObservedUploaded: observedUploaded, LastSeen: currentTimestamp, TorrentLastSeen: torrentLastSeen, Net: cidr, Port: clientPortMap, TorrentDownloaded: clientTorrentDownloadedMap, TorrentUploaded: clientTorrentUploadedMap}
	ipMapMutex.Unlock()
}
func IsIPTooHighUploaded(ipInfo IPInfoStruct, lastIPInfo IPInfoStruct) int64 {
	totalUploaded := IPUploadedDelta(ipInfo, lastIPInfo)

	if ConfigSnapshot().IPUpCheckIncrementMB > 0 {
		var totalUploadedMB int64 = (totalUploaded / 1024 / 1024)
		if totalUploadedMB > int64(ConfigSnapshot().IPUpCheckIncrementMB) {
			return totalUploadedMB
		}
	}

	return 0
}

// IPUploadedDelta excludes each connection's first sample from the observed interval.
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

func IsMatchCIDR(peerNet *net.IPNet) bool {
	if peerNet != nil {
		blockCIDRMapMutex.RLock()
		_, exist := blockCIDRMap[peerNet.String()]
		blockCIDRMapMutex.RUnlock()
		if exist {
			return true
		}
	}

	return false
}
func CheckAllIP(ipMap map[string]IPInfoStruct, lastIPMap map[string]IPInfoStruct) int {
	// 首轮同样检查端口数量并建立快照；上传增量只比较已有历史的 IP。
	if (ConfigSnapshot().MaxIPPortCount > 0 || (ConfigSnapshot().IPUploadedCheck && ConfigSnapshot().IPUpCheckIncrementMB > 0)) && currentTimestamp > (lastIPCleanTimestamp+int64(ConfigSnapshot().IPUpCheckInterval)) {
		ipBlockCount := 0

		ipMapMutex.Lock()
		lastIPMapMutex.Lock()
		defer ipMapMutex.Unlock()
		defer lastIPMapMutex.Unlock()

		// Keep connection history under real IPs; only the upload decision is grouped.
		uploadedByNetwork := make(map[string]int64)
		if ConfigSnapshot().IPUploadedCheck {
			for ip, info := range ipMap {
				if info.LastSeen > 0 && info.LastSeen <= lastIPCleanTimestamp {
					continue
				}
				if !IsBlockedPeer(ip, -1, false) {
					uploadedByNetwork[IPUploadGroup(ip)] += IPUploadedDelta(info, lastIPMap[ip])
				}
			}
		}

	ipMapLoop:
		for ip, ipInfo := range ipMap {
			if ipInfo.LastSeen > 0 && ipInfo.LastSeen <= lastIPCleanTimestamp {
				continue
			}
			if IsBlockedPeer(ip, -1, false) || len(ipInfo.Port) <= 0 {
				continue
			}

			for port := range ipInfo.Port {
				if IsBlockedPeer(ip, port, false) {
					continue ipMapLoop
				}
			}

			if ConfigSnapshot().MaxIPPortCount > 0 {
				if len(ipInfo.Port) > int(ConfigSnapshot().MaxIPPortCount) {
					Log("CheckAllIP_AddBlockPeer (Too many ports)", "%s:%d", true, ip, -1)
					ipBlockCount++
					BlockIPFromStatistics(ip, "Too many ports", ipInfo)
					continue
				}
			}

			if ConfigSnapshot().IPUpCheckIncrementMB > 0 {
				if uploadDuring := uploadedByNetwork[IPUploadGroup(ip)] / 1024 / 1024; uploadDuring > int64(ConfigSnapshot().IPUpCheckIncrementMB) {
					Log("CheckAllIP_AddBlockPeer (Global-Too high uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, ip, -1, uploadDuring)
					ipBlockCount++

					ipInfo.Net = ParseIPCIDRByConfig(ip)
					BlockIPFromStatistics(ip, "Global-Too high uploaded", ipInfo)
				}
			}
		}

		lastIPCleanTimestamp = currentTimestamp
		DeepCopyIPMap(ipMap, lastIPMap)

		return ipBlockCount
	}

	return 0
}

func IPUploadGroup(ip string) string {
	if cidr := ParseIPCIDRByConfig(ip); cidr != nil {
		return cidr.String()
	}
	return ip
}

// BlockIPFromStatistics keeps the initial total and its connection baselines in sync.
func BlockIPFromStatistics(ip, reason string, info IPInfoStruct) {
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
	AddBlockPeer("CheckAllIP", reason, ip, -1, "", "", "", downloaded, uploaded)
	SeedBlockedPeerCounters(ip, info.TorrentPeers)
	AddBlockCIDR(ip, info.Net)
}
