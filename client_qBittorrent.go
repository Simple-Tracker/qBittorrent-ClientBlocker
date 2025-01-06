package main

import (
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
)

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

func qB_GetClientConfigPath() string {
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
func qB_GetClientConfig() []byte {
	qBConfigFilename := qB_GetClientConfigPath()
	if qBConfigFilename == "" {
		return []byte{}
	}

	_, err := os.Stat(qBConfigFilename)
	if err != nil {
		if !os.IsNotExist(err) {
			// 避免反复猜测默认 qBittorrent 配置文件的失败信息影响 Debug 用户体验.
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
func qB_SetURL() bool {
	qBConfigFile := qB_GetClientConfig()
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
	if qBHTTPSEnabled {
		config.ClientURL = "https://" + qBAddress
		if qBPort != 443 {
			config.ClientURL += ":" + strconv.Itoa(qBPort)
		}
	} else {
		config.ClientURL = "http://" + qBAddress
		if qBPort != 80 {
			config.ClientURL += ":" + strconv.Itoa(qBPort)
		}
	}
	config.ClientURL += "/api"
	config.ClientUsername = Username
	Log("SetURL", GetLangText("Success-SetURL"), true, qBWebUIEnabled, config.ClientURL, config.ClientUsername)
	return true
}
func qB_GetAPIVersion() bool {
	if !strings.HasSuffix(config.ClientURL, "/api") {
		apiResponseStatusCodeWithSuffix, _, _ := Fetch(config.ClientURL+"/api/v2/app/webapiVersion", false, false, false, nil)
		if apiResponseStatusCodeWithSuffix == 200 || apiResponseStatusCodeWithSuffix == 403 {
			config.ClientURL += "/api"
			Log("qB_GetAPIVersion", GetLangText("ClientQB_Detect-OldClientURL"), true, config.ClientURL)
			return true
		}
	}

	apiResponseStatusCode, _, _ := Fetch(config.ClientURL+"/v2/app/webapiVersion", false, false, false, nil)
	return (apiResponseStatusCode == 200 || apiResponseStatusCode == 403)
}
func qB_Login() bool {
	loginParams := url.Values{}
	loginParams.Set("username", config.ClientUsername)
	loginParams.Set("password", config.ClientPassword)
	_, _, loginResponseBody := Submit(config.ClientURL+"/v2/auth/login", loginParams.Encode(), false, true, nil)
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
func qB_FetchTorrents() *[]qB_TorrentStruct {
	_, _, torrentsResponseBody := Fetch(config.ClientURL+"/v2/torrents/info?filter=active", true, true, false, nil)
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
func qB_FetchTorrentPeers(infoHash string) *qB_TorrentPeersStruct {
	_, _, torrentPeersResponseBody := Fetch(config.ClientURL+"/v2/sync/torrentPeers?rid=0&hash="+infoHash, true, true, false, nil)
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
func qB_SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
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
		_, _, banResponseBody = Submit(config.ClientURL+"/v2/transfer/banPeers", banIPPortsStr, true, true, nil)
	} else {
		banIPPortsStr = url.QueryEscape("{\"banned_IPs\": \"" + banIPPortsStr + "\"}")
		_, _, banResponseBody = Submit(config.ClientURL+"/v2/app/setPreferences", "json="+banIPPortsStr, true, true, nil)
	}

	if banResponseBody == nil {
		Log("SubmitBlockPeer", GetLangText("Error"), true)
		return false
	}

	return true
}

func qB_GetPreferences() map[string]interface{} {
	_, _, responseBody := Submit(config.ClientURL+"/v2/app/preferences", "", true, true, nil)
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
func qB_TestShadowBanAPI() bool {
	// 1. Check if enable_shadowban is true;
	// enable_shadowban may be not exist in the preferences.
	pref := qB_GetPreferences()
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

	// 2. Check if the API is available;
	code, _, _ := Submit(config.ClientURL+"/v2/transfer/shadowbanPeers", "peers=", true, true, nil)
	if code != 200 {
		Log("TestShadowBanAPI", GetLangText("Warning-ShadowBanAPINotExist"), true)
		return false
	}

	return true
}
func qB_SubmitShadowBanPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	shadowBanIPPortsList := []string{}
	for peerIP, peerInfo := range blockPeerMap {
		for port := range peerInfo.Port {
			if port <= 0 || port > 65535 {
				port = 1 // Seems qBittorrent will ignore the invalid port number, so we just set it to 1.
			}
			if IsIPv6(peerIP) { 
				shadowBanIPPortsList = append(shadowBanIPPortsList,  "[" + peerIP + "]:" + strconv.Itoa(port))
			} else {
				shadowBanIPPortsList = append(shadowBanIPPortsList, peerIP + ":" + strconv.Itoa(port))
				shadowBanIPPortsList = append(shadowBanIPPortsList, "[::ffff:" + peerIP + "]:" + strconv.Itoa(port))
			}
		}
	}

	banIPPortsStr := strings.Join(shadowBanIPPortsList, "|")
	Log("Debug-SubmitShadowBanPeer", "%s", false, banIPPortsStr)

	var banResponseBody []byte

	if banIPPortsStr != "" {
		banIPPortsStr = url.QueryEscape(banIPPortsStr)
		_, _, banResponseBody = Submit(config.ClientURL+"/v2/transfer/shadowbanPeers", "peers="+banIPPortsStr, true, true, nil)
	} else {
		return true
	}

	if banResponseBody == nil {
		Log("SubmitShadowBanPeer", GetLangText("Error"), true)
		return false
	}

	return true
}
