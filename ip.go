package main

import "net"

type IPInfoStruct struct {
	Net             *net.IPNet
	Port            map[int]bool
	TorrentUploaded map[string]int64
}

var ipMap = make(map[string]IPInfoStruct)
var lastIPMap = make(map[string]IPInfoStruct)
var lastIPCleanTimestamp int64 = 0

func AddIPInfo(cidr *net.IPNet, peerIP string, peerPort int, torrentInfoHash string, peerUploaded int64) {
	if !(config.MaxIPPortCount > 0 || (config.IPUploadedCheck && config.IPUpCheckIncrementMB > 0)) {
		return
	}

	var clientPortMap map[int]bool
	var clientTorrentUploadedMap map[string]int64

	if info, exist := ipMap[peerIP]; !exist {
		clientPortMap = make(map[int]bool)
		clientTorrentUploadedMap = make(map[string]int64)
	} else {
		clientPortMap = info.Port
		clientTorrentUploadedMap = info.TorrentUploaded
	}
	clientPortMap[peerPort] = true

	if oldPeerUploaded, exist := clientTorrentUploadedMap[torrentInfoHash]; !exist || oldPeerUploaded <= peerUploaded {
		clientTorrentUploadedMap[torrentInfoHash] = peerUploaded
	} else {
		clientTorrentUploadedMap[torrentInfoHash] += peerUploaded
	}

	ipMap[peerIP] = IPInfoStruct{Net: cidr, Port: clientPortMap, TorrentUploaded: clientTorrentUploadedMap}
}
func IsIPTooHighUploaded(ipInfo IPInfoStruct, lastIPInfo IPInfoStruct) int64 {
	var totalUploaded int64 = 0

	for torrentInfoHash, torrentUploaded := range ipInfo.TorrentUploaded {
		if config.IPUpCheckIncrementMB > 0 {
			if lastTorrentUploaded, exist := lastIPInfo.TorrentUploaded[torrentInfoHash]; !exist {
				totalUploaded += torrentUploaded
			} else {
				totalUploaded += (torrentUploaded - lastTorrentUploaded)
			}
		}
	}

	if config.IPUpCheckIncrementMB > 0 {
		var totalUploadedMB int64 = (totalUploaded / 1024 / 1024)
		if totalUploadedMB > int64(config.IPUpCheckIncrementMB) {
			return totalUploadedMB
		}
	}

	return 0
}
func IsMatchCIDR(peerNet *net.IPNet) bool {
	if peerNet != nil {
		if _, exist := blockCIDRMap[peerNet.String()]; exist {
			return true
		}
	}

	return false
}
func CheckAllIP(ipMap map[string]IPInfoStruct, lastIPMap map[string]IPInfoStruct) int {
	if (config.MaxIPPortCount > 0 || (config.IPUploadedCheck && config.IPUpCheckIncrementMB > 0)) && len(lastIPMap) > 0 && currentTimestamp > (lastIPCleanTimestamp+int64(config.IPUpCheckInterval)) {
		ipBlockCount := 0

	ipMapLoop:
		for ip, ipInfo := range ipMap {
			if IsBlockedPeer(ip, -1, true) || len(ipInfo.Port) <= 0 {
				continue
			}

			for port := range ipInfo.Port {
				if IsBlockedPeer(ip, port, true) {
					continue ipMapLoop
				}
			}

			if config.MaxIPPortCount > 0 {
				if len(ipInfo.Port) > int(config.MaxIPPortCount) {
					Log("CheckAllIP_AddBlockPeer (Too many ports)", "%s:%d", true, ip, -1)
					ipBlockCount++
					AddBlockPeer("CheckAllIP", "Too many ports", ip, -1, "")
					AddBlockCIDR(ip, ipInfo.Net)
					continue
				}
			}

			if lastIPInfo, exist := lastIPMap[ip]; exist {
				if uploadDuring := IsIPTooHighUploaded(ipInfo, lastIPInfo); uploadDuring > 0 {
					Log("CheckAllIP_AddBlockPeer (Global-Too high uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, ip, -1, uploadDuring)
					ipBlockCount++
					AddBlockPeer("CheckAllIP", "Global-Too high uploaded", ip, -1, "")
					AddBlockCIDR(ip, ipInfo.Net)
				}
			}
		}

		lastIPCleanTimestamp = currentTimestamp
		DeepCopyIPMap(ipMap, lastIPMap)

		return ipBlockCount
	}

	return 0
}
