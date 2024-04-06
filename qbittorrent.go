package main

import (
	"strings"
	"strconv"
	"encoding/json"
	"net/url"
)

type MainDataStruct struct {
	FullUpdate bool                     `json:"full_update"`
	Torrents   map[string]TorrentStruct `json:"torrents"`
}
type TorrentStruct struct {
	NumLeechs int64  `json:"num_leechs"`
	TotalSize int64  `json:"total_size"`
	Tracker   string `json:"tracker"`
}
type PeerStruct struct {
	IP             string
	Port           int
	Client         string
	Peer_ID_Client string
	Progress       float64
	Uploaded       int64
}
type TorrentPeersStruct struct {
	FullUpdate bool                  `json:"full_update"`
	Peers      map[string]PeerStruct `json:"peers"`
}

var useNewBanPeersMethod = false

func Login() bool {
	loginParams := url.Values {}
	loginParams.Set("username", config.QBUsername)
	loginParams.Set("password", config.QBPassword)
	loginResponseBody := Submit(config.QBURL + "/api/v2/auth/login", loginParams.Encode(), false, true)
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
func FetchMaindata() *MainDataStruct {
	maindataResponseBody := Fetch(config.QBURL + "/api/v2/sync/maindata?rid=0", true, true)
	if maindataResponseBody == nil {
		Log("FetchMaindata", GetLangText("Error"), true)
		return nil
	}

	var mainDataResult MainDataStruct
	if err := json.Unmarshal(maindataResponseBody, &mainDataResult); err != nil {
		Log("FetchMaindata", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	//Log("Debug-FetchMaindata", "完整更新: %s", false, strconv.FormatBool(mainDataResult.FullUpdate))

	return &mainDataResult
}
func FetchTorrentPeers(infoHash string) *TorrentPeersStruct {
	torrentPeersResponseBody := Fetch(config.QBURL + "/api/v2/sync/torrentPeers?rid=0&hash=" + infoHash, true, true)
	if torrentPeersResponseBody == nil {
		Log("FetchTorrentPeers", GetLangText("Error"), true)
		return nil
	}

	var torrentPeersResult TorrentPeersStruct
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
func GenBlockPeersStr(blockPeerMap map[string]BlockPeerInfoStruct) string {
	ip_ports := ""

	if useNewBanPeersMethod {
		for peerIP, peerInfo := range blockPeerMap {
			if _, exist := peerInfo.Port[-1]; config.BanAllPort || exist {
				for port := 0; port <= 65535; port++ {
					ip_ports += peerIP + ":" + strconv.Itoa(port) + "|"
				}
				continue
			}
			for port, _ := range peerInfo.Port {
				ip_ports += peerIP + ":" + strconv.Itoa(port) + "|"
			}
		}
		ip_ports = strings.TrimRight(ip_ports, "|")
	} else {
		for peerIP := range blockPeerMap {
			ip_ports += peerIP + "\n"
		}
	}

	return ip_ports
}
func SubmitBlockPeer(banIPPortsStr string) {
	var banResponseBody []byte

	if useNewBanPeersMethod && banIPPortsStr != "" {
		banIPPortsStr = url.QueryEscape(banIPPortsStr)
		banResponseBody = Submit(config.QBURL + "/api/v2/transfer/banPeers", banIPPortsStr, true, true)
	} else {
		banIPPortsStr = url.QueryEscape("{\"banned_IPs\": \"" + banIPPortsStr + "\"}")
		banResponseBody = Submit(config.QBURL + "/api/v2/app/setPreferences", "json=" + banIPPortsStr, true, true)
	}

	if banResponseBody == nil {
		Log("SubmitBlockPeer", GetLangText("Error"), true)
	}
}
