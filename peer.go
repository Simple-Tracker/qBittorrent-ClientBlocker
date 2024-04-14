package main

import (
	"net"
	"strings"
	"strconv"
)

type PeerInfoStruct struct {
	Net        *net.IPNet
	Port       map[int]bool
	Progress   float64
	Downloaded int64
	Uploaded   int64
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

	peerNet := ParseIPCIDRByConfig(peerIP)
	if peerNet != nil {
		peerNetStr := peerNet.String()
		blockCIDRMap[peerNetStr] = BlockCIDRInfoStruct { Timestamp: currentTimestamp, Net: peerNet }
	}

	if config.ExecCommand_Ban != "" {
		execCommand_Ban := config.ExecCommand_Ban
		execCommand_Ban = strings.Replace(execCommand_Ban, "{peerIP}", peerIP, -1)
		execCommand_Ban = strings.Replace(execCommand_Ban, "{peerPort}", strconv.Itoa(peerPort), -1)
		execCommand_Ban = strings.Replace(execCommand_Ban, "{torrentInfoHash}", torrentInfoHash, -1)
		out := ExecCommand(execCommand_Ban)

		if out != nil {
			Log("AddBlockPeer", GetLangText("Success-ExecCommand"), true, out)
		} else {
			Log("AddBlockPeer", GetLangText("Failed-ExecCommand"), true)
		}
	}
}
func ClearBlockPeer() int {
	cleanCount := 0
	if config.CleanInterval == 0 || (lastCleanTimestamp + int64(config.CleanInterval) < currentTimestamp) {
		for peerIP, peerInfo := range blockPeerMap {
			if currentTimestamp > (peerInfo.Timestamp + int64(config.BanTime)) {
				cleanCount++
				delete(blockPeerMap, peerIP)

				peerNet := ParseIPCIDRByConfig(peerIP)

				if peerNet != nil {
					peerNetStr := peerNet.String()
					if blockCIDRInfo, exist := blockCIDRMap[peerNetStr]; exist {
						if blockCIDRInfo.Timestamp > currentTimestamp {
							peerInfo.Timestamp = blockCIDRInfo.Timestamp
							blockPeerMap[peerIP] = peerInfo
							continue
						}
						delete(blockCIDRMap, peerNetStr)
					}
				}

				if config.ExecCommand_Unban != "" {
					for peerPort, _ := range peerInfo.Port {
						execCommand_Unban := config.ExecCommand_Unban
						execCommand_Unban = strings.Replace(execCommand_Unban, "{peerIP}", peerIP, -1)
						execCommand_Unban = strings.Replace(execCommand_Unban, "{peerPort}", strconv.Itoa(peerPort), -1)
						execCommand_Unban = strings.Replace(execCommand_Unban, "{torrentInfoHash}", peerInfo.InfoHash, -1)
						out := ExecCommand(execCommand_Unban)

						if out != nil {
							Log("AddBlockPeer", GetLangText("Success-ExecCommand"), true, out)
						} else {
							Log("AddBlockPeer", GetLangText("Failed-ExecCommand"), true)
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
func CheckPeer(peerIP string, peerPort int, peerID string, peerClient string, peerDlSpeed int64, peerUpSpeed int64, peerProgress float64, peerDownloaded int64, peerUploaded int64, torrentInfoHash string, torrentTotalSize int64) (int, *net.IPNet) {
	if peerIP == "" || CheckPrivateIP(peerIP) || (peerDlSpeed <= 0 && peerUpSpeed <= 0) {
		return -1, nil
	}

	if IsBlockedPeer(peerIP, peerPort, true) {
		Log("Debug-CheckPeer_IgnorePeer (Blocked)", "%s:%d %s|%s", false, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient))
		/*
		if peerPort == -2 {
			return 4
		}
		*/
		if peerPort == -1 {
			return 3, nil
		}
		return 2, nil
	}

	for port := range config.PortBlockList {
		if port == peerPort {
			Log("CheckPeer_AddBlockPeer (Bad-Port)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
			AddBlockPeer(peerIP, peerPort, torrentInfoHash)
			return 1, nil
		}
	}

	matchCIDR, peerNet := IsMatchCIDR(peerIP)
	if matchCIDR {
		Log("CheckPeer_AddBlockPeer (Bad-CIDR)", "%s:%d %s|%s (TorrentInfoHash: %s, Net: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, peerNet.String())
		AddBlockPeer(peerIP, peerPort, torrentInfoHash)
		return 1, peerNet
	}

	hasPeerClient := (peerID != "" || peerClient != "")
	ignoreByDownloaded := false
	// 若启用忽略且遇到空信息 Peer, 则既不会启用绝对进度屏蔽, 也不会记录 IP 及 Torrent 信息.
	if (!config.IgnoreEmptyPeer || hasPeerClient) {
		if (peerDownloaded / 1024 / 1024) >= int64(config.IgnoreByDownloaded) {
			ignoreByDownloaded = true
		}
		if !ignoreByDownloaded && IsProgressNotMatchUploaded(torrentTotalSize, peerProgress, peerUploaded) {
			Log("CheckPeer_AddBlockPeer (Bad-Progress_Uploaded)", "%s:%d %s|%s (TorrentInfoHash: %s, TorrentTotalSize: %.2f MB, Progress: %.2f%%, Uploaded: %.2f MB)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, (float64(torrentTotalSize) / 1024 / 1024), (peerProgress * 100), (float64(peerUploaded) / 1024 / 1024))
			AddBlockPeer(peerIP, peerPort, torrentInfoHash)
			return 1, peerNet
		}
	}

	if hasPeerClient {
		for _, v := range blockListCompiled {
			if v == nil {
				continue
			}
			if (peerClient != "" && v.MatchString(peerClient)) || (peerID != "" && v.MatchString(peerID)) {
				Log("CheckPeer_AddBlockPeer (Bad-Client_Normal)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, peerPort, torrentInfoHash)
				return 1, peerNet
			}
		}
		for _, v := range blockListFromURLCompiled {
			if v == nil {
				continue
			}
			if (peerClient != "" && v.MatchString(peerClient)) || (peerID != "" && v.MatchString(peerID)) {
				Log("CheckPeer_AddBlockPeer (Bad-Client_List)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, peerPort, torrentInfoHash)
				return 1, peerNet
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
				return 3, peerNet
			}
		}
		for _, v := range ipBlockListFromURLCompiled {
			if v == nil {
				continue
			}
			if v.Contains(ip) {
				Log("CheckPeer_AddBlockPeer (Bad-IP_Filter)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer(peerIP, -1, torrentInfoHash)
				return 3, peerNet
			}
		}
	}

	if (config.IgnoreEmptyPeer && !hasPeerClient) || ignoreByDownloaded {
		return -2, peerNet
	}

	return 0, peerNet
}
func ProcessPeer(peerIP string, peerPort int, peerID string, peerClient string, peerDlSpeed int64, peerUpSpeed int64, peerProgress float64, peerDownloaded int64, peerUploaded int64, torrentInfoHash string, torrentTotalSize int64, blockCount *int, ipBlockCount *int, badPeersCount *int, emptyPeersCount *int) {
	peerIP = ProcessIP(peerIP)
	peerStatus, peerNet := CheckPeer(peerIP, peerPort, peerID, peerClient, peerDlSpeed, peerUpSpeed, peerProgress, peerDownloaded, peerUploaded, torrentInfoHash, torrentTotalSize)
	if config.Debug_CheckPeer {
		Log("Debug-CheckPeer", "%s:%d %s|%s (TorrentInfoHash: %s, TorrentTotalSize: %d, PeerDlSpeed: %.2f%% MB/s, PeerUpSpeed: %.2f%% MB/s, Progress: %.2f%%, Downloaded: %.2f MB, Uploaded: %.2f MB, PeerStatus: %d)", false, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, torrentTotalSize, (float64(peerDlSpeed) / 1024 / 1024), (float64(peerUpSpeed) / 1024 / 1024), (peerProgress * 100), (float64(peerDownloaded) / 1024 / 1024), (float64(peerUploaded) / 1024 / 1024), peerStatus)
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
			if peerNet == nil {
				AddIPInfo(nil, peerIP, peerPort, torrentInfoHash, peerUploaded)
				AddTorrentInfo(torrentInfoHash, torrentTotalSize, nil, peerIP, peerPort, peerProgress, peerUploaded)
			} else {
				AddIPInfo(peerNet, peerNet.String(), peerPort, torrentInfoHash, peerUploaded)
				AddTorrentInfo(torrentInfoHash, torrentTotalSize, peerNet, peerNet.String(), peerPort, peerProgress, peerUploaded)
			}
	}
}
