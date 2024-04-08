package main

import (
	"strings"
	"strconv"
	"encoding/json"
	"net/url"
)

type qB_MainDataStruct struct {
	FullUpdate bool                        `json:"full_update"`
	Torrents   map[string]qB_TorrentStruct `json:"torrents"`
}
type qB_TorrentStruct struct {
	NumLeechs int64  `json:"num_leechs"`
	TotalSize int64  `json:"total_size"`
	Tracker   string `json:"tracker"`
}
type qB_PeerStruct struct {
	IP             string
	Port           int
	Client         string
	Peer_ID_Client string
	Progress       float64
	Uploaded       int64
}
type qB_TorrentPeersStruct struct {
	FullUpdate bool                     `json:"full_update"`
	Peers      map[string]qB_PeerStruct `json:"peers"`
}

var qB_useNewBanPeersMethod = false

func qB_Login() bool {
	loginParams := url.Values {}
	loginParams.Set("username", config.Username)
	loginParams.Set("password", config.Password)
	loginResponseBody := Submit(config.URL + "/api/v2/auth/login", loginParams.Encode(), false, true)
	if loginResponseBody == nil {
		Log("Login", GetLangText("Error-Login"), true)
		return false
	}

	loginResponseBodyStr := StrTrim(string(loginResponseBody))
	if loginResponseBodyStr == "Ok." {
		Log("Login", GetLangText("Success-Login"), true)
		return true
	} else if loginResponseBodyStr == "Fails." {
		Log("Login", GetLangText("Failed-Login_BadUsernameOrPassword"), true)
	} else {
		Log("Login", GetLangText("Failed-Login_Other"), true, loginResponseBodyStr)
	}
	return false
}
func qB_FetchMaindata() *qB_MainDataStruct {
	maindataResponseBody := Fetch(config.URL + "/api/v2/sync/maindata?rid=0", true, true)
	if maindataResponseBody == nil {
		Log("FetchMaindata", GetLangText("Error"), true)
		return nil
	}

	var mainDataResult qB_MainDataStruct
	if err := json.Unmarshal(maindataResponseBody, &mainDataResult); err != nil {
		Log("FetchMaindata", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	//Log("Debug-FetchMaindata", "完整更新: %s", false, strconv.FormatBool(mainDataResult.FullUpdate))

	return &mainDataResult
}
func qB_FetchTorrentPeers(infoHash string) *qB_TorrentPeersStruct {
	torrentPeersResponseBody := Fetch(config.URL + "/api/v2/sync/torrentPeers?rid=0&hash=" + infoHash, true, true)
	if torrentPeersResponseBody == nil {
		Log("FetchTorrentPeers", GetLangText("Error"), true)
		return nil
	}

	var torrentPeersResult qB_TorrentPeersStruct
	if err := json.Unmarshal(torrentPeersResponseBody, &torrentPeersResult); err != nil {
		Log("FetchTorrentPeers", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	/*
	if config.Debug_CheckTorrent {
		Log("Debug-FetchTorrentPeers", "完整更新: %s", false, strconv.FormatBool(torrentPeersResult.FullUpdate))
	}
	*/

	return &torrentPeersResult
}
func qB_SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) {
	banIPPortsStr := ""

	if blockPeerMap != nil {
		if qB_useNewBanPeersMethod {
			for peerIP, peerInfo := range blockPeerMap {
				if _, exist := peerInfo.Port[-1]; config.BanAllPort || exist {
					for port := 0; port <= 65535; port++ {
						if IsIPv6(peerIP) {
							banIPPortsStr += "[" + peerIP + "]:" + strconv.Itoa(port) + "|"
						} else {
							banIPPortsStr += peerIP + ":" + strconv.Itoa(port) + "|"
							banIPPortsStr += "[::ffff:" + peerIP + "]:" + strconv.Itoa(port) + "|"
						}
					}
					continue
				}
				for port, _ := range peerInfo.Port {
					banIPPortsStr += peerIP + ":" + strconv.Itoa(port) + "|"
				}
			}
			banIPPortsStr = strings.TrimRight(banIPPortsStr, "|")
		} else {
			for peerIP := range blockPeerMap {
				banIPPortsStr += peerIP + "\n"
				if !IsIPv6(peerIP) {
					banIPPortsStr += "::ffff:" + peerIP + "\n"
				}
			}
		}
	}

	Log("Debug-SubmitBlockPeer", "%s", false, banIPPortsStr)

	var banResponseBody []byte

	if qB_useNewBanPeersMethod && banIPPortsStr != "" {
		banIPPortsStr = url.QueryEscape(banIPPortsStr)
		banResponseBody = Submit(config.URL + "/api/v2/transfer/banPeers", banIPPortsStr, true, true)
	} else {
		banIPPortsStr = url.QueryEscape("{\"banned_IPs\": \"" + banIPPortsStr + "\"}")
		banResponseBody = Submit(config.URL + "/api/v2/app/setPreferences", "json=" + banIPPortsStr, true, true)
	}

	if banResponseBody == nil {
		Log("SubmitBlockPeer", GetLangText("Error"), true)
	}
}
