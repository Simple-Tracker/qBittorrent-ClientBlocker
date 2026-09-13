package main

import (
	"crypto/sha256"
	"encoding/json"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

// QBClient 实现了 qBittorrent 的客户端接口.
type QBClient struct {
	// qBittorrent 会话的 torrentPeers 游标跨种子共享，不能按 hash 独立缓存。
	peerRID        int
	peerCache      map[string]qB_PeerStruct
	peerURL        string
	peerFullSyncAt int64
	// 由主扫描循环使用；仅保留已成功提交的端口集合。
	banCache   map[string]map[int]bool
	banBatches map[[32]byte]bool
	banURL     string
	banAllPort bool
	banMethod  bool
}

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
	c.peerRID, c.peerCache = 0, nil
	c.banCache = nil
	c.banBatches = nil
	return QB_Login()
}

// FetchTorrents 获取所有活动的种子列表.
func (c *QBClient) FetchTorrents() ([]*Torrent, error) {
	torrents := QB_FetchTorrents()
	if torrents == nil {
		c.peerRID, c.peerCache = 0, nil
		c.banCache = nil
		c.banBatches = nil
		return nil, nil
	}
	result := make([]*Torrent, 0, len(*torrents))
	if len(*torrents) == 0 {
		c.peerRID, c.peerCache = 0, nil
	}
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
	peersStruct := c.FetchTorrentPeersResponse(torrent.Hash)
	if peersStruct == nil {
		return nil, nil
	}
	result := make([]*Peer, 0, len(peersStruct.Peers))
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

// 字段级合并：增量 JSON 中缺失的字段必须保留，显式零值必须覆盖。
func (c *QBClient) FetchTorrentPeersResponse(hash string) *qB_TorrentPeersStruct {
	url := ConfigSnapshot().ClientURL
	if c.peerURL != url || currentTimestamp-c.peerFullSyncAt >= 300 || currentTimestamp < c.peerFullSyncAt {
		c.peerRID, c.peerCache = 0, nil
	}
	rid := c.peerRID
	if rid == 0 {
		full := QB_FetchTorrentPeers(hash)
		if full == nil {
			c.peerRID, c.peerCache = 0, nil
			return nil
		}
		c.peerRID, c.peerCache = full.RID, full.Peers
		if c.peerCache == nil {
			c.peerCache = make(map[string]qB_PeerStruct)
		}
		c.peerURL, c.peerFullSyncAt = url, currentTimestamp
		return full
	}
	_, _, body := Fetch(url+"/v2/sync/torrentPeers?rid="+strconv.Itoa(rid)+"&hash="+hash, true, true, false, nil)
	if body == nil {
		c.peerRID, c.peerCache = 0, nil
		return nil
	}
	var response struct {
		RID        int                        `json:"rid"`
		FullUpdate bool                       `json:"full_update"`
		Peers      map[string]json.RawMessage `json:"peers"`
		Removed    []string                   `json:"peers_removed"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		c.peerRID, c.peerCache = 0, nil
		LogError("FetchTorrentPeers", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}
	reset := rid == 0 || response.FullUpdate
	updates := make(map[string]qB_PeerStruct, len(response.Peers))
	for key, raw := range response.Peers {
		peer := qB_PeerStruct{}
		if !reset {
			peer = c.peerCache[key]
		}
		if err := json.Unmarshal(raw, &peer); err != nil {
			c.peerRID, c.peerCache = 0, nil
			LogError("FetchTorrentPeers", GetLangText("Error-Parse"), true, err.Error())
			return nil
		}
		updates[key] = peer
	}
	if reset {
		c.peerCache = make(map[string]qB_PeerStruct)
		c.peerFullSyncAt = currentTimestamp
	}
	for _, key := range response.Removed {
		delete(c.peerCache, key)
	}
	for key, peer := range updates {
		c.peerCache[key] = peer
	}
	c.peerRID, c.peerURL = response.RID, url
	return &qB_TorrentPeersStruct{FullUpdate: true, Peers: c.peerCache}
}

func (c *QBClient) SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	cfg := ConfigSnapshot()
	if !qB_useNewBanPeersMethod {
		unchanged := !c.banMethod && c.banCache != nil && c.banURL == cfg.ClientURL && len(c.banCache) == len(blockPeerMap)
		for ip := range blockPeerMap {
			if _, exists := c.banCache[ip]; !exists {
				unchanged = false
				break
			}
		}
		if unchanged {
			return true
		}
		if !QB_SubmitBlockPeer(blockPeerMap) {
			return false
		}
		c.banCache = make(map[string]map[int]bool, len(blockPeerMap))
		for ip := range blockPeerMap {
			c.banCache[ip] = nil
		}
		c.banURL, c.banBatches, c.banMethod = cfg.ClientURL, nil, false
		return true
	}
	if len(blockPeerMap) == 0 {
		c.banCache, c.banBatches = nil, nil
		return QB_SubmitBlockPeer(blockPeerMap)
	}
	if !c.banMethod || c.banCache == nil || c.banURL != cfg.ClientURL || c.banAllPort != cfg.BanAllPort {
		c.banCache = make(map[string]map[int]bool)
		c.banBatches = nil
		c.banURL, c.banAllPort, c.banMethod = cfg.ClientURL, cfg.BanAllPort, true
	}
	for ip := range c.banCache {
		if _, exists := blockPeerMap[ip]; !exists {
			delete(c.banCache, ip)
		}
	}
	delta := make(map[string]BlockPeerInfoStruct)
	for ip, peer := range blockPeerMap {
		previous, exists := c.banCache[ip]
		ports := make(map[int]bool)
		for port := range peer.Port {
			if !previous[port] {
				ports[port] = true
			}
		}
		if !exists || len(ports) > 0 {
			if cfg.BanAllPort || peer.Port[-1] {
				ports = map[int]bool{-1: true}
			}
			peer.Port = ports
			delta[ip] = peer
		}
	}
	if len(delta) == 0 {
		return true
	}
	if c.banBatches == nil {
		c.banBatches = make(map[[32]byte]bool)
	}
	if !QB_submitBlockPeerBatches(delta, c.banBatches) {
		return false
	}
	c.banBatches = nil
	// 登录失败恢复可能在 Submit 内清空缓存。
	if c.banCache == nil {
		c.banCache = make(map[string]map[int]bool)
	}
	for ip := range delta {
		ports := make(map[int]bool)
		for port := range blockPeerMap[ip].Port {
			ports[port] = true
		}
		c.banCache[ip] = ports
	}
	return true
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
	RID        int                      `json:"rid"`
	FullUpdate bool                     `json:"full_update"`
	Peers      map[string]qB_PeerStruct `json:"peers"`
}

var qB_useNewBanPeersMethod = false

func QB_GetClientConfigPath() string {
	var qBConfigFilename string
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		LogError("Debug-GetClientConfigPath", GetLangText("Error-Debug-GetClientConfigPath_GetUserHomeDir"), true, err.Error())
		return ""
	}
	if IsUnix(userHomeDir) {
		qBConfigFilename = userHomeDir + "/.config/qBittorrent/qBittorrent.ini"
	} else {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			LogError("Debug-GetClientConfigPath", GetLangText("Error-Debug-GetClientConfigPath_GetUserConfigDir"), true, err.Error())
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
			LogError("GetClientConfig", GetLangText("Error-GetClientConfig_LoadConfigMeta"), true, err.Error())
		}
		return []byte{}
	}

	Log("GetClientConfig", GetLangText("GetClientConfig_UseConfig"), true, qBConfigFilename)

	qBConfigFile, err := os.ReadFile(qBConfigFilename)
	if err != nil {
		LogError("GetClientConfig", GetLangText("Error-GetClientConfig_LoadConfig"), true, err.Error())
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
		LogError("Login", GetLangText("Error-Login"), true)
		return false
	}

	loginResponseBodyStr := StrTrim(string(loginResponseBody))
	if loginResponseBodyStr == "Ok." {
		Log("Login", GetLangText("Success-Login"), true)
		return true
	} else if loginResponseBodyStr == "Fails." {
		LogError("Login", GetLangText("Failed-Login_BadUsernameOrPassword"), true)
	} else {
		LogError("Login", GetLangText("Failed-Login_Other"), true, loginResponseBodyStr)
	}
	return false
}
func QB_FetchTorrents() *[]qB_TorrentStruct {
	_, _, torrentsResponseBody := Fetch(ConfigSnapshot().ClientURL+"/v2/torrents/info?filter=active", true, true, false, nil)
	if torrentsResponseBody == nil {
		LogError("FetchTorrents", GetLangText("Error"), true)
		return nil
	}

	var torrentsResult []qB_TorrentStruct
	if err := json.Unmarshal(torrentsResponseBody, &torrentsResult); err != nil {
		LogError("FetchTorrents", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	return &torrentsResult
}
func QB_FetchTorrentPeers(infoHash string) *qB_TorrentPeersStruct {
	_, _, torrentPeersResponseBody := Fetch(ConfigSnapshot().ClientURL+"/v2/sync/torrentPeers?rid=0&hash="+infoHash, true, true, false, nil)
	if torrentPeersResponseBody == nil {
		LogError("FetchTorrentPeers", GetLangText("Error"), true)
		return nil
	}

	var torrentPeersResult qB_TorrentPeersStruct
	if err := json.Unmarshal(torrentPeersResponseBody, &torrentPeersResult); err != nil {
		LogError("FetchTorrentPeers", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	return &torrentPeersResult
}

// 每个表单限制为 256 KiB，避免全端口展开产生单个巨型请求。
const qBMaxBanFormBytes = 256 * 1024

func QB_SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	return QB_submitBlockPeerBatches(blockPeerMap, nil)
}

func QB_submitBlockPeerBatches(blockPeerMap map[string]BlockPeerInfoStruct, completed map[[32]byte]bool) bool {
	cfg := ConfigSnapshot()
	if qB_useNewBanPeersMethod && len(blockPeerMap) > 0 {
		var form strings.Builder
		form.WriteString("peers=")
		flush := func() bool {
			if form.Len() == len("peers=") {
				return true
			}
			batchKey := sha256.Sum256([]byte(form.String()))
			if !completed[batchKey] {
				_, _, body := Submit(cfg.ClientURL+"/v2/transfer/banPeers", form.String(), true, true, nil)
				if body == nil {
					return false
				}
				if completed != nil {
					completed[batchKey] = true
				}
			}
			form.Reset()
			form.WriteString("peers=")
			return true
		}
		writePeer := func(peer string) bool {
			encoded := url.QueryEscape(peer)
			if form.Len()+len(encoded)+3 > qBMaxBanFormBytes && !flush() {
				return false
			}
			if form.Len() > len("peers=") {
				form.WriteString("%7C")
			}
			form.WriteString(encoded)
			return true
		}
		ips := make([]string, 0, len(blockPeerMap))
		for ip := range blockPeerMap {
			ips = append(ips, ip)
		}
		sort.Strings(ips)
		for _, ip := range ips {
			peer := blockPeerMap[ip]
			address := ip
			if IsIPv6(ip) {
				address = "[" + ip + "]"
			}
			if cfg.BanAllPort || peer.Port[-1] {
				for port := 0; port <= 65535; port++ {
					suffix := ":" + strconv.Itoa(port)
					if !writePeer(address + suffix) {
						return false
					}
					if !IsIPv6(ip) && !writePeer("[::ffff:"+ip+"]"+suffix) {
						return false
					}
				}
			} else {
				ports := make([]int, 0, len(peer.Port))
				for port := range peer.Port {
					ports = append(ports, port)
				}
				sort.Ints(ports)
				for _, port := range ports {
					if !writePeer(address + ":" + strconv.Itoa(port)) {
						return false
					}
				}
			}
		}
		return flush()
	}
	// setPreferences 是覆盖语义，必须保留完整 IP 名单并正确转义 JSON 换行。
	var ips strings.Builder
	for ip := range blockPeerMap {
		ips.WriteString(ip + "\n")
		if !IsIPv6(ip) {
			ips.WriteString("::ffff:" + ip + "\n")
		}
	}
	data, err := json.Marshal(map[string]string{"banned_IPs": ips.String()})
	if err != nil {
		return false
	}
	_, _, body := Submit(cfg.ClientURL+"/v2/app/setPreferences", "json="+url.QueryEscape(string(data)), true, true, nil)
	return body != nil
}

func QB_GetPreferences() map[string]interface{} {
	_, _, responseBody := Fetch(ConfigSnapshot().ClientURL+"/v2/app/preferences", true, true, false, nil)
	if responseBody == nil {
		LogError("GetPreferences", GetLangText("Failed-GetQBPreferences"), true)
		return nil
	}

	var preferences map[string]interface{}
	if err := json.Unmarshal(responseBody, &preferences); err != nil {
		LogError("GetPreferences", GetLangText("Error-Parse"), true, err.Error())
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
		LogError("TestShadowBanAPI", GetLangText("Failed-UnknownShadowBanAPI"), true)
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
		LogError("SubmitShadowBanPeer", GetLangText("Error"), true)
		return false
	}

	return true
}
