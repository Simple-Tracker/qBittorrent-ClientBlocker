package main

import (
	"net"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
)

type BlockPeerInfoStruct struct {
	Timestamp int64
	Module    string
	Reason    string
	Port      map[int]bool
	InfoHash  string
}
type BlockCIDRInfoStruct struct {
	Timestamp int64
	Net       *net.IPNet
	IPs       map[string]bool
}

var lastCleanTimestamp int64 = 0
var blockPeerMap = make(map[string]BlockPeerInfoStruct)
var blockCIDRMap = make(map[string]BlockCIDRInfoStruct)

func AddBlockPeer(module string, reason string, peerIP string, peerPort int, torrentInfoHash string) {
	var blockPeerPortMap map[int]bool
	if blockPeer, exist := blockPeerMap[peerIP]; !exist {
		blockPeerPortMap = make(map[int]bool)
	} else {
		blockPeerPortMap = blockPeer.Port
	}

	blockPeerPortMap[peerPort] = true
	blockPeerMap[peerIP] = BlockPeerInfoStruct{Timestamp: currentTimestamp, Module: module, Reason: reason, Port: blockPeerPortMap, InfoHash: torrentInfoHash}

	AddBlockCIDR(peerIP, ParseIPCIDRByConfig(peerIP))

	if config.ExecCommand_Ban != "" {
		execCommand_Ban := config.ExecCommand_Ban
		execCommand_Ban = strings.Replace(execCommand_Ban, "{peerIP}", peerIP, -1)
		execCommand_Ban = strings.Replace(execCommand_Ban, "{peerPort}", strconv.Itoa(peerPort), -1)
		execCommand_Ban = strings.Replace(execCommand_Ban, "{torrentInfoHash}", torrentInfoHash, -1)
		status, out, err := ExecCommand(execCommand_Ban)

		if status {
			Log("AddBlockPeer", GetLangText("Success-ExecCommand"), true, out)
		} else {
			Log("AddBlockPeer", GetLangText("Failed-ExecCommand"), true, out, err)
		}
	}
}
func AddBlockCIDR(peerIP string, peerNet *net.IPNet) {
	if peerNet == nil {
		return
	}

	peerNetStr := peerNet.String()
	var blockIPsMap map[string]bool
	if blockCIDRInfo, exist := blockCIDRMap[peerNetStr]; !exist {
		blockIPsMap = make(map[string]bool)
	} else {
		blockIPsMap = blockCIDRMap[peerNetStr].IPs
		if _, exist := blockCIDRInfo.IPs[peerIP]; !exist {
			blockIPsMap[peerIP] = true
		}
	}

	blockCIDRMap[peerNetStr] = BlockCIDRInfoStruct{Timestamp: currentTimestamp, Net: peerNet, IPs: blockIPsMap}
}
func ClearBlockPeer() int {
	cleanCount := 0
	if blockPeerMap != nil && config.CleanInterval == 0 || (lastCleanTimestamp+int64(config.CleanInterval) < currentTimestamp) {
		for peerIP, peerInfo := range blockPeerMap {
			if currentTimestamp > (peerInfo.Timestamp + int64(config.BanTime)) {
				cleanCount++
				delete(blockPeerMap, peerIP)

				peerNet := ParseIPCIDRByConfig(peerIP)

				if peerNet != nil {
					peerNetStr := peerNet.String()
					if blockCIDRInfo, exist := blockCIDRMap[peerNetStr]; exist {
						if blockCIDRInfo.Timestamp > peerInfo.Timestamp {
							peerInfo.Timestamp = blockCIDRInfo.Timestamp
							blockPeerMap[peerIP] = peerInfo
							continue
						}

						delete(blockCIDRInfo.IPs, peerIP)
						if len(blockCIDRInfo.IPs) <= 0 {
							delete(blockCIDRMap, peerNetStr)
							continue
						}

						blockCIDRMap[peerNetStr] = blockCIDRInfo
					}
				}

				if config.ExecCommand_Unban != "" {
					for peerPort, _ := range peerInfo.Port {
						execCommand_Unban := config.ExecCommand_Unban
						execCommand_Unban = strings.Replace(execCommand_Unban, "{peerIP}", peerIP, -1)
						execCommand_Unban = strings.Replace(execCommand_Unban, "{peerPort}", strconv.Itoa(peerPort), -1)
						execCommand_Unban = strings.Replace(execCommand_Unban, "{torrentInfoHash}", peerInfo.InfoHash, -1)
						status, out, err := ExecCommand(execCommand_Unban)

						if status {
							Log("AddBlockPeer", GetLangText("Success-ExecCommand"), true, out)
						} else {
							Log("AddBlockPeer", GetLangText("Failed-ExecCommand"), true, out, err)
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
func MatchBlockList(blockRegex *regexp2.Regexp, peerIP string, peerPort int, peerID string, peerClient string) bool {
	if blockRegex != nil {
		if peerClient != "" {
			isMatchPeerClient, err := blockRegex.MatchString(peerClient)

			if err != nil {
				Log("MatchBlockList_PeerClient", GetLangText("Error-MatchRegexpErr"), true, err.Error())
			} else if isMatchPeerClient {
				return true
			}
		}

		if peerID != "" {
			isMatchPeerID, err := blockRegex.MatchString(peerID)

			if err != nil {
				Log("MatchBlockList_PeerID", GetLangText("Error-MatchRegexpErr"), true, err.Error())
			} else if isMatchPeerID {
				return true
			}
		}
	}

	return false
}
func CheckPeer(peerIP string, peerPort int, peerID, peerClient string, peerDlSpeed, peerUpSpeed int64, peerProgress float64, peerDownloaded, peerUploaded int64, torrentInfoHash string, torrentTotalSize int64) (int, *net.IPNet) {
	if peerIP == "" || CheckPrivateIP(peerIP) {
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

	peerNet := ParseIPCIDRByConfig(peerIP)
	hasPeerClient := (peerID != "" || peerClient != "")

	if hasPeerClient {
		earlyStop := false
		blockListCompiled.Range(func(key, val any) bool {
			if MatchBlockList(val.(*regexp2.Regexp), peerIP, peerPort, peerID, peerClient) {
				Log("CheckPeer_AddBlockPeer (Bad-Client_Normal)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer("CheckPeer", "Bad-Client_Normal", peerIP, peerPort, torrentInfoHash)
				earlyStop = true
				return false
			}
			return true
		})

		if earlyStop {
			return 1, peerNet
		}
	}

	for port := range config.PortBlockList {
		if port == peerPort {
			Log("CheckPeer_AddBlockPeer (Bad-Port)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
			AddBlockPeer("CheckPeer", "Bad-Port", peerIP, peerPort, torrentInfoHash)
			return 1, peerNet
		}
	}

	ip := net.ParseIP(peerIP)
	if ip == nil {
		Log("Debug-CheckPeer_AddBlockPeer (Bad-IP)", "%s:%d %s|%s (TorrentInfoHash: %s)", false, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
	} else {
		earlyStop := false
		ipBlockListCompiled.Range(func(_, v any) bool {
			if v == nil {
				return true
			}

			ipNet, ok := (v).(*net.IPNet)
			if !ok {
				return true
			}
			if ipNet.Contains(ip) {
				Log("CheckPeer_AddBlockPeer (Bad-IP_Normal)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer("CheckPeer", "Bad-IP_Normal", peerIP, -1, torrentInfoHash)
				earlyStop = true
				return false
			}

			return true
		})
		if earlyStop {
			return 3, peerNet
		}

		for _, v := range ipBlockCIDRMapFromSyncServerCompiled {
			if v == nil {
				continue
			}
			if v.Contains(ip) {
				Log("CheckPeer_AddBlockPeer (Bad-IP_FromSyncServer)", "%s:%d %s|%s (TorrentInfoHash: %s)", true, peerIP, -1, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash)
				AddBlockPeer("CheckPeer", "Bad-IP_FromSyncServer", peerIP, -1, torrentInfoHash)
				return 3, peerNet
			}
		}
	}

	if IsMatchCIDR(peerNet) {
		Log("CheckPeer_AddBlockPeer (Bad-CIDR)", "%s:%d %s|%s (TorrentInfoHash: %s, PeerNet: %s)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, peerNet.String())
		AddBlockPeer("CheckPeer", "Bad-CIDR", peerIP, peerPort, torrentInfoHash)
		return 1, peerNet
	}

	if peerDlSpeed <= 0 && peerUpSpeed <= 0 {
		return -2, peerNet
	}

	ignoreByDownloaded := false
	// 若启用忽略且遇到空信息 Peer, 则既不会启用绝对进度屏蔽, 也不会记录 IP 及 Torrent 信息.
	if !config.IgnoreEmptyPeer || hasPeerClient {
		if config.IgnoreByDownloaded > 0 && (peerDownloaded/1024/1024) >= int64(config.IgnoreByDownloaded) {
			ignoreByDownloaded = true
		}
		if !ignoreByDownloaded && IsProgressNotMatchUploaded(torrentTotalSize, peerProgress, peerUploaded) {
			Log("CheckPeer_AddBlockPeer (Bad-Progress_Uploaded)", "%s:%d %s|%s (TorrentInfoHash: %s, TorrentTotalSize: %.2f MB, PeerDlSpeed: %.2f MB/s, PeerUpSpeed: %.2f MB/s, Progress: %.2f%%, Downloaded: %.2f MB, Uploaded: %.2f MB)", true, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, (float64(torrentTotalSize) / 1024 / 1024), (float64(peerDlSpeed) / 1024 / 1024), (float64(peerUpSpeed) / 1024 / 1024), (peerProgress * 100), (float64(peerDownloaded) / 1024 / 1024), (float64(peerUploaded) / 1024 / 1024))
			AddBlockPeer("CheckPeer", "Bad-Progress_Uploaded", peerIP, peerPort, torrentInfoHash)
			return 1, peerNet
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
		Log("Debug-CheckPeer", "%s:%d %s|%s (TorrentInfoHash: %s, TorrentTotalSize: %.2f MB, PeerDlSpeed: %.2f MB/s, PeerUpSpeed: %.2f MB/s, Progress: %.2f%%, Downloaded: %.2f MB, Uploaded: %.2f MB, PeerStatus: %d)", false, peerIP, peerPort, strconv.QuoteToASCII(peerID), strconv.QuoteToASCII(peerClient), torrentInfoHash, (float64(torrentTotalSize) / 1024 / 1024), (float64(peerDlSpeed) / 1024 / 1024), (float64(peerUpSpeed) / 1024 / 1024), (peerProgress * 100), (float64(peerDownloaded) / 1024 / 1024), (float64(peerUploaded) / 1024 / 1024), peerStatus)
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
