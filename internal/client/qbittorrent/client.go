package qbittorrent

import (
	"crypto/sha256"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

// Client 实现了 qBittorrent 的客户端接口.
type Client struct {
	services client.Services
	// qBittorrent 会话的 torrentPeers 游标跨种子共享, 不能按 hash 独立缓存.
	peerRID        int
	peerCache      map[string]PeerRecord
	peerURL        string
	peerFullSyncAt int64
	// 由主扫描循环使用; 仅保留已成功提交的端口集合.
	banCache   map[string]map[int]bool
	banBatches map[[32]byte]bool
	banURL     string
	banAllPort bool
	banMethod  bool
}

func (c *Client) GetClientType() string {
	return "qBittorrent"
}

func (c *Client) ConfigPath() string {
	return c.configPath()
}

func (c *Client) SetURL() bool {
	return c.setURL()
}

func (c *Client) Login() bool {
	c.peerRID, c.peerCache = 0, nil
	c.banCache = nil
	c.banBatches = nil
	return c.login()
}

// FetchTorrents 获取所有活动的种子列表.
func (c *Client) FetchTorrents() ([]*client.Torrent, error) {
	torrents := c.fetchTorrents()
	if torrents == nil {
		c.peerRID, c.peerCache = 0, nil
		c.banCache = nil
		c.banBatches = nil
		return nil, nil
	}
	result := make([]*client.Torrent, 0, len(*torrents))
	if len(*torrents) == 0 {
		c.peerRID, c.peerCache = 0, nil
	}
	for _, t := range *torrents {
		result = append(result, &client.Torrent{
			Hash:       t.InfoHash,
			Tracker:    t.Tracker,
			LeechCount: t.NumLeechs,
			TotalSize:  t.TotalSize,
		})
	}
	return result, nil
}

// FetchTorrentPeers 获取特定种子的 client.Peer 列表.
func (c *Client) FetchTorrentPeers(torrent *client.Torrent) ([]*client.Peer, error) {
	peersStruct := c.FetchTorrentPeersResponse(torrent.Hash)
	if peersStruct == nil {
		return nil, nil
	}
	result := make([]*client.Peer, 0, len(peersStruct.Peers))
	for _, p := range peersStruct.Peers {
		result = append(result, &client.Peer{
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

// 字段级合并: 增量 JSON 中缺失的字段必须保留, 显式零值必须覆盖.
func (c *Client) FetchTorrentPeersResponse(hash string) *TorrentPeersResponse {
	url := c.services.Snapshot().URL
	if c.peerURL != url || c.services.Now()-c.peerFullSyncAt >= 300 || c.services.Now() < c.peerFullSyncAt {
		c.peerRID, c.peerCache = 0, nil
	}
	rid := c.peerRID
	if rid == 0 {
		full := c.fetchTorrentPeers(hash)
		if full == nil {
			c.peerRID, c.peerCache = 0, nil
			return nil
		}
		c.peerRID, c.peerCache = full.RID, full.Peers
		if c.peerCache == nil {
			c.peerCache = make(map[string]PeerRecord)
		}
		c.peerURL, c.peerFullSyncAt = url, c.services.Now()
		return full
	}
	_, _, body := c.services.Fetch(url+"/v2/sync/torrentPeers?rid="+strconv.Itoa(rid)+"&hash="+hash, true, true, false, nil)
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
		c.services.LogError("FetchTorrentPeers", c.services.Text("Error-Parse"), true, err.Error())
		return nil
	}
	reset := rid == 0 || response.FullUpdate
	updates := make(map[string]PeerRecord, len(response.Peers))
	for key, raw := range response.Peers {
		peer := PeerRecord{}
		if !reset {
			peer = c.peerCache[key]
		}
		if err := json.Unmarshal(raw, &peer); err != nil {
			c.peerRID, c.peerCache = 0, nil
			c.services.LogError("FetchTorrentPeers", c.services.Text("Error-Parse"), true, err.Error())
			return nil
		}
		updates[key] = peer
	}
	if reset {
		c.peerCache = make(map[string]PeerRecord)
		c.peerFullSyncAt = c.services.Now()
	}
	for _, key := range response.Removed {
		delete(c.peerCache, key)
	}
	for key, peer := range updates {
		c.peerCache[key] = peer
	}
	c.peerRID, c.peerURL = response.RID, url
	return &TorrentPeersResponse{FullUpdate: true, Peers: c.peerCache}
}

func (c *Client) SubmitBlockPeer(blockPeerMap map[string]client.BanTarget) bool {
	if !c.validateBlockPeerIPs(blockPeerMap) {
		return false
	}
	cfg := c.services.Snapshot()
	if !c.services.Snapshot().NewBanPeersMethod {
		unchanged := !c.banMethod && c.banCache != nil && c.banURL == cfg.URL && len(c.banCache) == len(blockPeerMap)
		for ip := range blockPeerMap {
			if _, exists := c.banCache[ip]; !exists {
				unchanged = false
				break
			}
		}
		if unchanged {
			return true
		}
		if !c.submitBlockPeer(blockPeerMap) {
			return false
		}
		c.banCache = make(map[string]map[int]bool, len(blockPeerMap))
		for ip := range blockPeerMap {
			c.banCache[ip] = nil
		}
		c.banURL, c.banBatches, c.banMethod = cfg.URL, nil, false
		return true
	}
	if len(blockPeerMap) == 0 {
		c.banCache, c.banBatches = nil, nil
		return c.submitBlockPeer(blockPeerMap)
	}
	if !c.banMethod || c.banCache == nil || c.banURL != cfg.URL || c.banAllPort != cfg.BanAllPort {
		c.banCache = make(map[string]map[int]bool)
		c.banBatches = nil
		c.banURL, c.banAllPort, c.banMethod = cfg.URL, cfg.BanAllPort, true
	}
	for ip := range c.banCache {
		if _, exists := blockPeerMap[ip]; !exists {
			delete(c.banCache, ip)
		}
	}
	delta := make(map[string]client.BanTarget)
	for ip, peer := range blockPeerMap {
		previous, exists := c.banCache[ip]
		ports := make(map[int]bool)
		for port := range peer.Ports {
			if !previous[port] {
				ports[port] = true
			}
		}
		if !exists || len(ports) > 0 {
			if cfg.BanAllPort || peer.Ports[-1] {
				ports = map[int]bool{-1: true}
			}
			peer.Ports = ports
			delta[ip] = peer
		}
	}
	if len(delta) == 0 {
		return true
	}
	if c.banBatches == nil {
		c.banBatches = make(map[[32]byte]bool)
	}
	if !c.submitBlockPeerBatches(delta, c.banBatches) {
		return false
	}
	c.banBatches = nil
	// 登录失败恢复可能在 Submit 内清空缓存.
	if c.banCache == nil {
		c.banCache = make(map[string]map[int]bool)
	}
	for ip := range delta {
		ports := make(map[int]bool)
		for port := range blockPeerMap[ip].Ports {
			ports[port] = true
		}
		c.banCache[ip] = ports
	}
	return true
}

func (c *Client) SubmitShadowBanPeer(blockPeerMap map[string]client.BanTarget) bool {
	return c.submitShadowBanPeer(blockPeerMap)
}

type TorrentRecord struct {
	InfoHash  string `json:"hash"`
	NumLeechs int64  `json:"num_leechs"`
	TotalSize int64  `json:"total_size"`
	Tracker   string `json:"tracker"`
}
type PeerRecord struct {
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
type TorrentPeersResponse struct {
	RID        int                   `json:"rid"`
	FullUpdate bool                  `json:"full_update"`
	Peers      map[string]PeerRecord `json:"peers"`
}

func New(services client.Services) *Client { return &Client{services: services.WithDefaults()} }

func (c *Client) configPath() string {
	var qBConfigFilename string
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		c.services.LogError("Debug-GetClientConfigPath", c.services.Text("Error-Debug-GetClientConfigPath_GetUserHomeDir"), true, err.Error())
		return ""
	}
	if !strings.Contains(userHomeDir, "\\") {
		qBConfigFilename = userHomeDir + "/.config/qBittorrent/qBittorrent.ini"
	} else {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			c.services.LogError("Debug-GetClientConfigPath", c.services.Text("Error-Debug-GetClientConfigPath_GetUserConfigDir"), true, err.Error())
			return ""
		}
		qBConfigFilename = userConfigDir + "\\qBittorrent\\qBittorrent.ini"
	}
	return qBConfigFilename
}
func (c *Client) configFile() []byte {
	qBConfigFilename := c.configPath()
	if qBConfigFilename == "" {
		return []byte{}
	}

	_, err := os.Stat(qBConfigFilename)
	if err != nil {
		if !os.IsNotExist(err) {
			c.services.LogError("GetClientConfig", c.services.Text("Error-GetClientConfig_LoadConfigMeta"), true, err.Error())
		}
		return []byte{}
	}

	c.services.Log("GetClientConfig", c.services.Text("GetClientConfig_UseConfig"), true, qBConfigFilename)

	qBConfigFile, err := os.ReadFile(qBConfigFilename)
	if err != nil {
		c.services.LogError("GetClientConfig", c.services.Text("Error-GetClientConfig_LoadConfig"), true, err.Error())
		return []byte{}
	}

	return qBConfigFile
}
func (c *Client) setURL() bool {
	qBConfigFile := c.configFile()
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
		qbConfigLineArr[0] = strings.ToLower(client.Trim(qbConfigLineArr[0]))
		qbConfigLineArr[1] = strings.ToLower(client.Trim(qbConfigLineArr[1]))
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
		c.services.Log("SetURL", c.services.Text("Abandon-SetURL"), true, qBWebUIEnabled, qBAddress)
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
	c.services.SetEndpoint(clientURL, Username)
	c.services.Log("SetURL", c.services.Text("Success-SetURL"), true, qBWebUIEnabled, clientURL, Username)
	return true
}
func (c *Client) Detect() bool {
	clientURL := c.services.Snapshot().URL
	if !strings.HasSuffix(clientURL, "/api") {
		apiResponseStatusCodeWithSuffix, _, _ := c.services.Fetch(clientURL+"/api/v2/app/webapiVersion", false, false, false, nil)
		if apiResponseStatusCodeWithSuffix == 200 || apiResponseStatusCodeWithSuffix == 403 {
			clientURL += "/api"
			c.services.SetEndpoint(clientURL, c.services.Snapshot().Username)
			c.services.Log("qB_GetAPIVersion", c.services.Text("ClientQB_Detect-OldClientURL"), true, clientURL)
			return true
		}
	}

	apiResponseStatusCode, _, _ := c.services.Fetch(clientURL+"/v2/app/webapiVersion", false, false, false, nil)
	return (apiResponseStatusCode == 200 || apiResponseStatusCode == 403)
}
func (c *Client) login() bool {
	loginParams := url.Values{}
	loginParams.Set("username", c.services.Snapshot().Username)
	loginParams.Set("password", c.services.Snapshot().Password)
	loginResponseCode, _, loginResponseBody := c.services.Submit(c.services.Snapshot().URL+"/v2/auth/login", loginParams.Encode(), false, true, nil)

	// 参考: https://github.com/Simple-Tracker/qBittorrent-ClientBlocker/issues/159
	if loginResponseCode == 204 {
		c.services.Log("Login", c.services.Text("Success-Login"), true)
		return true
	}

	if loginResponseBody == nil {
		c.services.LogError("Login", c.services.Text("Error-Login"), true)
		return false
	}

	loginResponseBodyStr := client.Trim(string(loginResponseBody))
	if loginResponseBodyStr == "Ok." {
		c.services.Log("Login", c.services.Text("Success-Login"), true)
		return true
	} else if loginResponseBodyStr == "Fails." {
		c.services.LogError("Login", c.services.Text("Failed-Login_BadUsernameOrPassword"), true)
	} else {
		c.services.LogError("Login", c.services.Text("Failed-Login_Other"), true, loginResponseBodyStr)
	}
	return false
}
func (c *Client) fetchTorrents() *[]TorrentRecord {
	_, _, torrentsResponseBody := c.services.Fetch(c.services.Snapshot().URL+"/v2/torrents/info?filter=active", true, true, false, nil)
	if torrentsResponseBody == nil {
		c.services.LogError("FetchTorrents", c.services.Text("Error"), true)
		return nil
	}

	var torrentsResult []TorrentRecord
	if err := json.Unmarshal(torrentsResponseBody, &torrentsResult); err != nil {
		c.services.LogError("FetchTorrents", c.services.Text("Error-Parse"), true, err.Error())
		return nil
	}

	return &torrentsResult
}
func (c *Client) fetchTorrentPeers(infoHash string) *TorrentPeersResponse {
	_, _, torrentPeersResponseBody := c.services.Fetch(c.services.Snapshot().URL+"/v2/sync/torrentPeers?rid=0&hash="+infoHash, true, true, false, nil)
	if torrentPeersResponseBody == nil {
		c.services.LogError("FetchTorrentPeers", c.services.Text("Error"), true)
		return nil
	}

	var torrentPeersResult TorrentPeersResponse
	if err := json.Unmarshal(torrentPeersResponseBody, &torrentPeersResult); err != nil {
		c.services.LogError("FetchTorrentPeers", c.services.Text("Error-Parse"), true, err.Error())
		return nil
	}

	return &torrentPeersResult
}

// 每个表单限制为 256 KiB, 避免全端口展开产生单个巨型请求.
const qBMaxBanFormBytes = 256 * 1024

// 先验证整份名单, 避免分批提交到一半才发现 CIDR 或无效地址.
func (c *Client) validateBlockPeerIPs(peers map[string]client.BanTarget) bool {
	for ip := range peers {
		if net.ParseIP(ip) == nil {
			c.services.LogError("c.submitBlockPeer", "Invalid peer IP: %q", true, ip)
			return false
		}
	}
	return true
}

func (c *Client) submitBlockPeer(blockPeerMap map[string]client.BanTarget) bool {
	return c.submitBlockPeerBatches(blockPeerMap, nil)
}

func (c *Client) submitBlockPeerBatches(blockPeerMap map[string]client.BanTarget, completed map[[32]byte]bool) bool {
	if !c.validateBlockPeerIPs(blockPeerMap) {
		return false
	}
	cfg := c.services.Snapshot()
	if c.services.Snapshot().NewBanPeersMethod && len(blockPeerMap) > 0 {
		var form strings.Builder
		form.WriteString("peers=")
		flush := func() bool {
			if form.Len() == len("peers=") {
				return true
			}
			batchKey := sha256.Sum256([]byte(form.String()))
			if !completed[batchKey] {
				_, _, body := c.services.Submit(cfg.URL+"/v2/transfer/banPeers", form.String(), true, true, nil)
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
			if client.IsIPv6(ip) {
				address = "[" + ip + "]"
			}
			if cfg.BanAllPort || peer.Ports[-1] {
				for port := 0; port <= 65535; port++ {
					suffix := ":" + strconv.Itoa(port)
					if !writePeer(address + suffix) {
						return false
					}
					if !client.IsIPv6(ip) && !writePeer("[::ffff:"+ip+"]"+suffix) {
						return false
					}
				}
			} else {
				ports := make([]int, 0, len(peer.Ports))
				for port := range peer.Ports {
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
	// setPreferences 是覆盖语义, 必须保留完整 IP 名单并正确转义 JSON 换行.
	var ips strings.Builder
	for ip := range blockPeerMap {
		ips.WriteString(ip + "\n")
		if !client.IsIPv6(ip) {
			ips.WriteString("::ffff:" + ip + "\n")
		}
	}
	data, err := json.Marshal(map[string]string{"banned_IPs": ips.String()})
	if err != nil {
		return false
	}
	_, _, body := c.services.Submit(cfg.URL+"/v2/app/setPreferences", "json="+url.QueryEscape(string(data)), true, true, nil)
	return body != nil
}

func (c *Client) preferences() map[string]interface{} {
	_, _, responseBody := c.services.Fetch(c.services.Snapshot().URL+"/v2/app/preferences", true, true, false, nil)
	if responseBody == nil {
		c.services.LogError("GetPreferences", c.services.Text("Failed-GetQBPreferences"), true)
		return nil
	}

	var preferences map[string]interface{}
	if err := json.Unmarshal(responseBody, &preferences); err != nil {
		c.services.LogError("GetPreferences", c.services.Text("Error-Parse"), true, err.Error())
		return nil
	}

	return preferences
}
func (c *Client) TestShadowBanAPI() bool {
	pref := c.preferences()
	if pref == nil {
		return false
	}

	enableShadowBan, exist := pref["shadow_ban_enabled"]
	if !exist {
		c.services.Log("TestShadowBanAPI", c.services.Text("Warning-ShadowBanAPINotExist"), true)
		return false
	}

	if bEnableShadowBan, ok := enableShadowBan.(bool); ok {
		if !bEnableShadowBan {
			return false
		}
	} else {
		c.services.LogError("TestShadowBanAPI", c.services.Text("Failed-UnknownShadowBanAPI"), true)
		return false
	}

	code, _, _ := c.services.Submit(c.services.Snapshot().URL+"/v2/transfer/shadowbanPeers", "peers=", true, true, nil)
	if code != 200 {
		c.services.Log("TestShadowBanAPI", c.services.Text("Warning-ShadowBanAPINotExist"), true)
		return false
	}

	return true
}
func (c *Client) submitShadowBanPeer(blockPeerMap map[string]client.BanTarget) bool {
	if !c.validateBlockPeerIPs(blockPeerMap) {
		return false
	}
	shadowBanIPPortsList := []string{}
	for peerIP, peerInfo := range blockPeerMap {
		for port := range peerInfo.Ports {
			if port <= 0 || port > 65535 {
				port = 1
			}
			if client.IsIPv6(peerIP) {
				shadowBanIPPortsList = append(shadowBanIPPortsList, "["+peerIP+"]:"+strconv.Itoa(port))
			} else {
				shadowBanIPPortsList = append(shadowBanIPPortsList, peerIP+":"+strconv.Itoa(port))
				shadowBanIPPortsList = append(shadowBanIPPortsList, "[::ffff:"+peerIP+"]:"+strconv.Itoa(port))
			}
		}
	}

	banIPPortsStr := strings.Join(shadowBanIPPortsList, "|")
	c.services.Log("Debug-SubmitShadowBanPeer", "%s", false, banIPPortsStr)

	var banResponseBody []byte

	if banIPPortsStr != "" {
		banIPPortsStr = url.QueryEscape(banIPPortsStr)
		_, _, banResponseBody = c.services.Submit(c.services.Snapshot().URL+"/v2/transfer/shadowbanPeers", "peers="+banIPPortsStr, true, true, nil)
	} else {
		return true
	}

	if banResponseBody == nil {
		c.services.LogError("SubmitShadowBanPeer", c.services.Text("Error"), true)
		return false
	}

	return true
}
