package main

import (
	"net"
	"strconv"
)

type PeerInfoStruct struct {
	Port     map[int]bool
	Progress float64
	Uploaded int64
}
type BlockPeerInfoStruct struct {
	Timestamp int64
	Port      map[int]bool
	InfoHash  string
}

var lastCleanTimestamp int64 = 0
var blockPeerMap = make(map[string]BlockPeerInfoStruct)
var blockCIDRMap = make(map[string]BlockCIDRInfoStruct)

func AddBlockPeer(peerIP string, peerPort int, torrentInfoHash string) {
	var blockPeerPortMap map[int]bool
	if blockPeer, exist := blockPeerMap[peerIP]; !exist {
		blockPeerPortMap = make(map[int]bool)
	} else {
		blockPeerPortMap = blockPeer.Port
	}

	blockPeerPortMap[peerPort] = true
	blockPeerMap[peerIP] = BlockPeerInfoStruct { Timestamp: currentTimestamp, Port: blockPeerPortMap, InfoHash: torrentInfoHash }

	cidr := ""
	if IsIPv6(peerIP) {
		if config.BanIP6CIDR != "/128" {
			cidr = config.BanIP6CIDR
		}
	} else {
		if config.BanIPCIDR != "/32" {
			cidr = config.BanIPCIDR
		}
	}

	if cidr != "" {
		peerNet := ParseIP(peerIP + cidr)
		if peerNet != nil {
			peerNetStr := peerNet.String()
			blockCIDRMap[peerNetStr] = BlockCIDRInfoStruct { Timestamp: currentTimestamp, Net: peerNet }
		}
	}
}
func ClearBlockPeer() int {
	cleanCount := 0
	if config.CleanInterval == 0 || (lastCleanTimestamp + int64(config.CleanInterval) < currentTimestamp) {
		for clientIP, clientInfo := range blockPeerMap {
			if currentTimestamp > (clientInfo.Timestamp + int64(config.BanTime)) {
				cleanCount++
				delete(blockPeerMap, clientIP)

				cidr := ""
				if IsIPv6(clientIP) {
					if config.BanIP6CIDR != "/128" {
						cidr = config.BanIP6CIDR
					}
				} else {
					if config.BanIPCIDR != "/32" {
						cidr = config.BanIPCIDR
					}
				}

				if cidr != "" {
					peerNet := ParseIP(clientIP + cidr)
					if peerNet != nil {
						peerNetStr := peerNet.String()
						if blockCIDRInfo, exist := blockCIDRMap[peerNetStr]; exist {
							if blockCIDRInfo.Timestamp > currentTimestamp {
								clientInfo.Timestamp = blockCIDRInfo.Timestamp
								blockPeerMap[clientIP] = clientInfo
								continue
							}
							delete(blockCIDRMap, peerNetStr)
						}
					}
				}
			}
		}
		if cleanCount != 0 {
			lastCleanTimestamp = currentTimestamp
			Log("ClearBlockPeer", GetLangText("Success-ClearBlockPeer"), true, cleanCount)
		}
	}

	return cleanCount
}
func IsBlockedPeer(peerIP string, peerPort int, updateTimestamp bool) bool {
	if blockPeer, exist := blockPeerMap[peerIP]; exist {
		if IsBanPort() {
			if _, exist1 := blockPeer.Port[-1]; !exist1 {
				if _, exist2 := blockPeer.Port[peerPort]; !exist2 {
					return false
				}
			}
		}

		if updateTimestamp {
			blockPeer.Timestamp = currentTimestamp
			blockPeerMap[peerIP] = blockPeer
		}

		return true
	}
	
	return false
}
func CheckPeer(peerIP string, peerPort int, peerID string, peerClient string, peerProgress float64, peerUploaded int64, torrentInfoHash string, torrentTotalSize int64) int {
	if peerIP == "" || CheckPrivateIP(peerIP) {
		return -1
	}

	if IsBlockedPeer(peerIP, peerPort, true) {
		Log("Debug-CheckPeer_IgnorePeer (Blocked)", "%s:%d %s|%s", false, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient))
		/*
		if peerPort == -2 {
			return 4
		}
		*/
		if peerPort == -1 {
			return 3
		}
		return 2
	}

	for port := range config.PortBlockList {
		if port == peerPort {
			Log("CheckPeer_AddBlockPeer (Bad-Port)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
			AddBlockPeer(peerIP, peerPort, torrentInfoHash)
			return 1
		}
	}

	peerNetStr := IsMatchCIDR(peerIP)
	if peerNetStr != "" {
		Log("CheckPeer_AddBlockPeer (Bad-CIDR)", "%s:%d %s|%s (TorrentInfoHash: %s, Net: %s)", false, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, peerNetStr)
		AddBlockPeer(peerIP, peerPort, torrentInfoHash)
		return 1
	}

	hasPeerClient := (peerID != "" || peerClient != "")
	// 若启用忽略且遇到空信息 Peer, 则既不会启用绝对进度屏蔽, 也不会记录 IP 及 Torrent 信息.
	if (!config.IgnoreEmptyPeer || hasPeerClient) && IsProgressNotMatchUploaded(torrentTotalSize, peerProgress, peerUploaded) {
		Log("CheckPeer_AddBlockPeer (Bad-Progress_Uploaded)", "%s:%d %s|%s (TorrentInfoHash: %s, TorrentTotalSize: %.2f MB, Progress: %.2f%%, Uploaded: %.2f MB)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, (float64(torrentTotalSize) / 1024 / 1024), (peerProgress * 100), (float64(peerUploaded) / 1024 / 1024))
		AddBlockPeer(peerIP, peerPort, torrentInfoHash)
		return 1
	}

	if hasPeerClient {
		for _, v := range blockListCompiled {
			if v == nil {
				continue
			}
			if (peerClient != "" && v.MatchString(peerClient)) || (peerID != "" && v.MatchString(peerID)) {
				Log("CheckPeer_AddBlockPeer (Bad-Client_Normal)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, peerPort, torrentInfoHash)
				return 1
			}
		}
		for _, v := range blockListFromURLCompiled {
			if v == nil {
				continue
			}
			if (peerClient != "" && v.MatchString(peerClient)) || (peerID != "" && v.MatchString(peerID)) {
				Log("CheckPeer_AddBlockPeer (Bad-Client_List)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, peerPort, torrentInfoHash)
				return 1
			}
		}
	}

	ip := net.ParseIP(peerIP)
	if ip == nil {
		Log("Debug-CheckPeer_AddBlockPeer (Bad-IP)", "%s:%d %s|%s (TorrentInfoHash: %s)", false, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
	} else {
		for _, v := range ipBlockListCompiled {
			if v == nil {
				continue
			}
			if v.Contains(ip) {
				Log("CheckPeer_AddBlockPeer (Bad-IP_Normal)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, -1, torrentInfoHash)
				return 3
			}
		}
		for _, v := range ipBlockListFromURLCompiled {
			if v == nil {
				continue
			}
			if v.Contains(ip) {
				Log("CheckPeer_AddBlockPeer (Bad-IP_Filter)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, -1, torrentInfoHash)
				return 3
			}
		}
	}

	if (config.IgnoreEmptyPeer && !hasPeerClient) {
		return -2
	}

	return 0
}
func ProcessPeer(peerIP string, peerPort int, peerID string, peerClient string, peerProgress float64, peerUploaded int64, torrentInfoHash string, torrentTotalSize int64, blockCount *int, ipBlockCount *int, badPeersCount *int, emptyPeersCount *int) {
	peerIP = ProcessIP(peerIP)
	peerStatus := CheckPeer(peerIP, peerPort, peerID, peerClient, peerProgress, peerUploaded, torrentInfoHash, torrentTotalSize)
	if config.Debug_CheckPeer {
		Log("Debug-CheckPeer", "%s:%d %s|%s (Status: %d)", false, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), peerStatus)
	}

	switch peerStatus {
		case 1:
			*blockCount++
		case 3:
			*ipBlockCount++
		case -1:
			*badPeersCount++
		case -2:
			*emptyPeersCount++
		case 0:
			AddIPInfo(peerIP, peerPort, torrentInfoHash, peerUploaded)
			AddTorrentInfo(torrentInfoHash, torrentTotalSize, peerIP, peerPort, peerProgress, peerUploaded)
	}
}
