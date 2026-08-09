package main

import (
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// QBClient 实现了 qBittorrent 的客户端接口.
type QBClient struct{}

func (c *QBClient) GetClientType() string {
	return "qBittorrent"
}

func (c *QBClient) ConfigPath() string {
	return QB_GetClientConfigPath()
}

func (c *QBClient) SetURL() bool {
	return QB_SetURL()
}

func (c *QBClient) Login() bool {
	return QB_Login()
}

// FetchTorrents 获取所有活动的种子列表.
func (c *QBClient) FetchTorrents() ([]*Torrent, error) {
	torrents := QB_FetchTorrents()
	if torrents == nil {
		return nil, nil
	}
	var result []*Torrent
	for _, t := range *torrents {
		result = append(result, &Torrent{
			Hash:       t.InfoHash,
			Tracker:    t.Tracker,
			LeechCount: t.NumLeechs,
			TotalSize:  t.TotalSize,
		})
	}
	return result, nil
}

// FetchTorrentPeers 获取特定种子的 Peer 列表.
func (c *QBClient) FetchTorrentPeers(torrent *Torrent) ([]*Peer, error) {
	peersStruct := QB_FetchTorrentPeers(torrent.Hash)
	if peersStruct == nil {
		return nil, nil
	}
	var result []*Peer
	for _, p := range peersStruct.Peers {
		result = append(result, &Peer{
			IP:         p.IP,
			Port:       p.Port,
			ID:         p.PeerID,
			Client:     p.Client,
			DlSpeed:    p.DlSpeed,
			UpSpeed:    p.UpSpeed,
			Progress:   p.Progress,
			Downloaded: p.Downloaded,
			Uploaded:   p.Uploaded,
		})
	}
	return result, nil
}

func (c *QBClient) SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	return QB_SubmitBlockPeer(blockPeerMap)
}

func (c *QBClient) SubmitShadowBanPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	return QB_SubmitShadowBanPeer(blockPeerMap)
}

type qB_TorrentStruct struct {
	InfoHash  string `json:"hash"`
	NumLeechs int64  `json:"num_leechs"`
	TotalSize int64  `json:"total_size"`
	Tracker   string `json:"tracker"`
}
type qB_PeerStruct struct {
	IP         string  `json:"ip"`
	Port       int     `json:"port"`
	Client     string  `json:"client"`
	PeerID     string  `json:"peer_id_client"`
	Progress   float64 `json:"progress"`
	Downloaded int64   `json:"downloaded"`
	Uploaded   int64   `json:"uploaded"`
	DlSpeed    int64   `json:"dl_speed"`
	UpSpeed    int64   `json:"up_speed"`
}
type qB_TorrentPeersStruct struct {
	FullUpdate bool                     `json:"full_update"`
	Peers      map[string]qB_PeerStruct `json:"peers"`
}

var qB_useNewBanPeersMethod = false

func QB_GetClientConfigPath() string {
	var qBConfigFilename string
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		Log("Debug-GetClientConfigPath", GetLangText("Error-Debug-GetClientConfigPath_GetUserHomeDir"), true, err.Error())
		return ""
	}
	if IsUnix(userHomeDir) {
		qBConfigFilename = userHomeDir + "/.config/qBittorrent/qBittorrent.ini"
	} else {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			Log("Debug-GetClientConfigPath", GetLangText("Error-Debug-GetClientConfigPath_GetUserConfigDir"), true, err.Error())
			return ""
		}
		qBConfigFilename = userConfigDir + "\\qBittorrent\\qBittorrent.ini"
	}
	return qBConfigFilename
}
func QB_GetClientConfig() []byte {
	qBConfigFilename := QB_GetClientConfigPath()
	if qBConfigFilename == "" {
		return []byte{}
	}

	_, err := os.Stat(qBConfigFilename)
	if err != nil {
		if !os.IsNotExist(err) {
			Log("GetClientConfig", GetLangText("Error-GetClientConfig_LoadConfigMeta"), true, err.Error())
		}
		return []byte{}
	}

	Log("GetClientConfig", GetLangText("GetClientConfig_UseConfig"), true, qBConfigFilename)

	qBConfigFile, err := os.ReadFile(qBConfigFilename)
	if err != nil {
		Log("GetClientConfig", GetLangText("Error-GetClientConfig_LoadConfig"), true, err.Error())
		return []byte{}
	}

	return qBConfigFile
}
func QB_SetURL() bool {
	qBConfigFile := QB_GetClientConfig()
	if len(qBConfigFile) < 1 {
		return false
	}
	qBConfigFileArr := strings.Split(string(qBConfigFile), "\n")
	qBWebUIEnabled := false
	qBHTTPSEnabled := false
	qBAddress := ""
	qBPort := 8080
	Username := ""
	for _, qbConfigLine := range qBConfigFileArr {
		qbConfigLineArr := strings.SplitN(qbConfigLine, "=", 2)
		if len(qbConfigLineArr) < 2 || qbConfigLineArr[1] == "" {
			continue
		}
		qbConfigLineArr[0] = strings.ToLower(StrTrim(qbConfigLineArr[0]))
		qbConfigLineArr[1] = strings.ToLower(StrTrim(qbConfigLineArr[1]))
		switch qbConfigLineArr[0] {
		case "webui\\enabled":
			if qbConfigLineArr[1] == "true" {
				qBWebUIEnabled = true
			}
		case "webui\\https\\enabled":
			if qbConfigLineArr[1] == "true" {
				qBHTTPSEnabled = true
			}
		case "webui\\address":
			if qbConfigLineArr[1] == "*" || qbConfigLineArr[1] == "0.0.0.0" {
				qBAddress = "127.0.0.1"
			} else if qbConfigLineArr[1] == "::" || qbConfigLineArr[1] == "::1" {
				qBAddress = "[::1]"
			} else {
				qBAddress = qbConfigLineArr[1]
			}
		case "webui\\port":
			tmpQBPort, err := strconv.Atoi(qbConfigLineArr[1])
			if err == nil {
				qBPort = tmpQBPort
			}
		case "webui\\username":
			Username = qbConfigLineArr[1]
		}
	}
	if !qBWebUIEnabled || qBAddress == "" {
		Log("SetURL", GetLangText("Abandon-SetURL"), true, qBWebUIEnabled, qBAddress)
		return false
	}
	clientURL := ""
	if qBHTTPSEnabled {
		clientURL = "https://" + qBAddress
		if qBPort != 443 {
			clientURL += ":" + strconv.Itoa(qBPort)
		}
	} else {
		clientURL = "http://" + qBAddress
		if qBPort != 80 {
			clientURL += ":" + strconv.Itoa(qBPort)
		}
	}
	clientURL += "/api"
	UpdateConfig(func(newConfig *ConfigStruct) {
		newConfig.ClientURL = clientURL
		newConfig.ClientUsername = Username
	})
	Log("SetURL", GetLangText("Success-SetURL"), true, qBWebUIEnabled, clientURL, Username)
	return true
}
func (c *QBClient) Detect() bool {
	clientURL := ConfigSnapshot().ClientURL
	if !strings.HasSuffix(clientURL, "/api") {
		apiResponseStatusCodeWithSuffix, _, _ := Fetch(clientURL+"/api/v2/app/webapiVersion", false, false, false, nil)
		if apiResponseStatusCodeWithSuffix == 200 || apiResponseStatusCodeWithSuffix == 403 {
			clientURL += "/api"
			UpdateConfig(func(newConfig *ConfigStruct) {
				newConfig.ClientURL = clientURL
			})
			Log("qB_GetAPIVersion", GetLangText("ClientQB_Detect-OldClientURL"), true, clientURL)
			return true
		}
	}

	apiResponseStatusCode, _, _ := Fetch(clientURL+"/v2/app/webapiVersion", false, false, false, nil)
	return (apiResponseStatusCode == 200 || apiResponseStatusCode == 403)
}
func QB_Login() bool {
	loginParams := url.Values{}
	loginParams.Set("username", ConfigSnapshot().ClientUsername)
	loginParams.Set("password", ConfigSnapshot().ClientPassword)
	loginResponseCode, _, loginResponseBody := Submit(ConfigSnapshot().ClientURL+"/v2/auth/login", loginParams.Encode(), false, true, nil)

	// see: https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/issues/159
	if loginResponseCode == 204 {
		Log("Login", GetLangText("Success-Login"), true)
		return true
	}

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
func QB_FetchTorrents() *[]qB_TorrentStruct {
	_, _, torrentsResponseBody := Fetch(ConfigSnapshot().ClientURL+"/v2/torrents/info?filter=active", true, true, false, nil)
	if torrentsResponseBody == nil {
		Log("FetchTorrents", GetLangText("Error"), true)
		return nil
	}

	var torrentsResult []qB_TorrentStruct
	if err := json.Unmarshal(torrentsResponseBody, &torrentsResult); err != nil {
		Log("FetchTorrents", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	return &torrentsResult
}
func QB_FetchTorrentPeers(infoHash string) *qB_TorrentPeersStruct {
	_, _, torrentPeersResponseBody := Fetch(ConfigSnapshot().ClientURL+"/v2/sync/torrentPeers?rid=0&hash="+infoHash, true, true, false, nil)
	if torrentPeersResponseBody == nil {
		Log("FetchTorrentPeers", GetLangText("Error"), true)
		return nil
	}

	var torrentPeersResult qB_TorrentPeersStruct
	if err := json.Unmarshal(torrentPeersResponseBody, &torrentPeersResult); err != nil {
		Log("FetchTorrentPeers", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	return &torrentPeersResult
}
func QB_SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	var banIPPortsBuilder strings.Builder
	banIPPortsBuilder.Grow(len(blockPeerMap) * 32)

	if blockPeerMap != nil {
		if qB_useNewBanPeersMethod {
			firstPeer := true
			writePeer := func(peer string) {
				if !firstPeer {
					banIPPortsBuilder.WriteByte('|')
				}
				banIPPortsBuilder.WriteString(peer)
				firstPeer = false
			}
			for peerIP, peerInfo := range blockPeerMap {
				if _, exist := peerInfo.Port[-1]; ConfigSnapshot().BanAllPort || exist {
					for port := 0; port <= 65535; port++ {
						portString := strconv.Itoa(port)
						if IsIPv6(peerIP) {
							writePeer("[" + peerIP + "]:" + portString)
						} else {
							writePeer(peerIP + ":" + portString)
							writePeer("[::ffff:" + peerIP + "]:" + portString)
						}
					}
					continue
				}
				for port := range peerInfo.Port {
					if IsIPv6(peerIP) {
						writePeer("[" + peerIP + "]:" + strconv.Itoa(port))
					} else {
						writePeer(peerIP + ":" + strconv.Itoa(port))
					}
				}
			}
		} else {
			for peerIP := range blockPeerMap {
				banIPPortsBuilder.WriteString(peerIP)
				banIPPortsBuilder.WriteByte('\n')
				if !IsIPv6(peerIP) {
					banIPPortsBuilder.WriteString("::ffff:")
					banIPPortsBuilder.WriteString(peerIP)
					banIPPortsBuilder.WriteByte('\n')
				}
			}
		}
	}
	banIPPortsStr := banIPPortsBuilder.String()

	Log("Debug-SubmitBlockPeer", "%s", false, banIPPortsStr)

	var banResponseBody []byte

	if qB_useNewBanPeersMethod && banIPPortsStr != "" {
		banIPPortsStr = url.QueryEscape(banIPPortsStr)
		_, _, banResponseBody = Submit(ConfigSnapshot().ClientURL+"/v2/transfer/banPeers", "peers="+banIPPortsStr, true, true, nil)
	} else {
		banIPPortsStr = url.QueryEscape("{\"banned_IPs\": \"" + banIPPortsStr + "\"}")
		_, _, banResponseBody = Submit(ConfigSnapshot().ClientURL+"/v2/app/setPreferences", "json="+banIPPortsStr, true, true, nil)
	}

	if banResponseBody == nil {
		Log("SubmitBlockPeer", GetLangText("Error"), true)
		return false
	}

	return true
}

func QB_GetPreferences() map[string]interface{} {
	_, _, responseBody := Fetch(ConfigSnapshot().ClientURL+"/v2/app/preferences", true, true, false, nil)
	if responseBody == nil {
		Log("GetPreferences", GetLangText("Failed-GetQBPreferences"), true)
		return nil
	}

	var preferences map[string]interface{}
	if err := json.Unmarshal(responseBody, &preferences); err != nil {
		Log("GetPreferences", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	return preferences
}
func QB_TestShadowBanAPI() bool {
	pref := QB_GetPreferences()
	if pref == nil {
		return false
	}

	enableShadowBan, exist := pref["shadow_ban_enabled"]
	if !exist {
		Log("TestShadowBanAPI", GetLangText("Warning-ShadowBanAPINotExist"), true)
		return false
	}

	if bEnableShadowBan, ok := enableShadowBan.(bool); ok {
		if !bEnableShadowBan {
			return false
		}
	} else {
		Log("TestShadowBanAPI", GetLangText("Failed-UnknownShadowBanAPI"), true)
		return false
	}

	code, _, _ := Submit(ConfigSnapshot().ClientURL+"/v2/transfer/shadowbanPeers", "peers=", true, true, nil)
	if code != 200 {
		Log("TestShadowBanAPI", GetLangText("Warning-ShadowBanAPINotExist"), true)
		return false
	}

	return true
}
func QB_SubmitShadowBanPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	shadowBanIPPortsList := []string{}
	for peerIP, peerInfo := range blockPeerMap {
		for port := range peerInfo.Port {
			if port <= 0 || port > 65535 {
				port = 1
			}
			if IsIPv6(peerIP) {
				shadowBanIPPortsList = append(shadowBanIPPortsList, "["+peerIP+"]:"+strconv.Itoa(port))
			} else {
				shadowBanIPPortsList = append(shadowBanIPPortsList, peerIP+":"+strconv.Itoa(port))
				shadowBanIPPortsList = append(shadowBanIPPortsList, "[::ffff:"+peerIP+"]:"+strconv.Itoa(port))
			}
		}
	}

	banIPPortsStr := strings.Join(shadowBanIPPortsList, "|")
	Log("Debug-SubmitShadowBanPeer", "%s", false, banIPPortsStr)

	var banResponseBody []byte

	if banIPPortsStr != "" {
		banIPPortsStr = url.QueryEscape(banIPPortsStr)
		_, _, banResponseBody = Submit(ConfigSnapshot().ClientURL+"/v2/transfer/shadowbanPeers", "peers="+banIPPortsStr, true, true, nil)
	} else {
		return true
	}

	if banResponseBody == nil {
		Log("SubmitShadowBanPeer", GetLangText("Error"), true)
		return false
	}

	return true
}
