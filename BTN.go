package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dlclark/regexp2"
	"github.com/tidwall/jsonc"
)

type BTN_Ability struct {
	Interval           uint32 `json:"interval"`
	Endpoint           string `json:"endpoint"`
	RandomInitialDelay uint32 `json:"random_initial_delay"`
	Version            string `json:"version"`
}

type BTN_ConfigStruct struct {
	MinMainVersion uint32                 `json:"min_protocol_version"`
	MaxMainVersion uint32                 `json:"max_protocol_version"`
	Ability        map[string]BTN_Ability `json:"ability"`
}

type BTN_PeerInternalStruct struct {
	IPAddress          string  `json:"ip_address"`
	PeerPort           int     `json:"peer_port"`
	PeerID             string  `json:"peer_id"`
	ClientName         string  `json:"client_name"`
	TorrentIdentifier  string  `json:"torrent_identifier"`
	TorrentSize        int64   `json:"torrent_size"`
	TorrentIsPrivate   bool    `json:"torrent_is_private"`
	Downloaded         int64   `json:"downloaded"`
	RTDownloadSpeed    int64   `json:"rt_download_speed"`
	Uploaded           int64   `json:"uploaded"`
	RTUploadSpeed      int64   `json:"rt_upload_speed"`
	PeerProgress       float64 `json:"peer_progress"`
	DownloaderProgress float64 `json:"downloader_progress"`
	PeerFlag           string  `json:"peer_flag"`
}

type BTN_SubmitPeersStruct struct {
	PopulateTime int64                    `json:"populate_time"`
	Peers        []BTN_PeerInternalStruct `json:"peers"`
}

type BTN_PeerHistoryStruct struct {
	BTN_PeerInternalStruct
	DownloadedOffset int64 `json:"downloaded_offset"`
	UploadedOffset   int64 `json:"uploaded_offset"`
	FirstTimeSeen    int64 `json:"first_time_seen"`
	LastTimeSeen     int64 `json:"last_time_seen"`
}

type BTN_SubmitHistoriesStruct struct {
	PopulateTime int64                    `json:"populate_time"`
	Peers        []BTN_PeerHistoryStruct `json:"peers"`
}

type BTN_BanInfo struct {
	BtnBan       bool                   `json:"btn_ban"`
	Module       string                 `json:"module"`
	Rule         string                 `json:"rule"`
	Peer         BTN_PeerInternalStruct `json:"peer"`
	BanAt        int64                  `json:"ban_at"`
	BanUniqueID  string                 `json:"ban_unique_id"`
}

type BTN_SubmitBansStruct struct {
	PopulateTime int64         `json:"populate_time"`
	Bans         []BTN_BanInfo `json:"bans"`
}

type BTN_RulesStruct struct {
	Version    string              `json:"version"`
	PeerID     map[string][]string `json:"peer_id"`
	ClientName map[string][]string `json:"client_name"`
	IP         map[string][]string `json:"ip"`
	Port       map[string][]string `json:"port"`
}

type RuleEntry struct {
	Method  string `json:"method"`
	Content string `json:"content"`
}

type BTN_ExceptionStruct struct {
	Version    string              `json:"version"`
	PeerID     map[string][]string `json:"peer_id"`
	ClientName map[string][]string `json:"client_name"`
	IP         map[string][]string `json:"ip"`
	Port       map[string][]string `json:"port"`
}

var btnProtocol = "BTN-Protocol/0.0.0-dev"
var btnUserAgent = programUserAgent + " " + btnProtocol

var btn_lastTaskExecution = make(map[string]int64)
var btn_taskInitialDelay = make(map[string]int64)
var btn_taskMutex sync.Mutex

func getBTNAuthHeader() map[string]string {
	h := map[string]string{
		"User-Agent": btnUserAgent,
	}
	if config.BTNAppID != "" && config.BTNAppSecret != "" {
		h["Authorization"] = "Bearer " + config.BTNAppID + "@" + config.BTNAppSecret
		h["X-BTN-AppID"] = config.BTNAppID
		h["X-BTN-AppSecret"] = config.BTNAppSecret
	}
	return h
}

func getBTNSubmitHeader() map[string]string {
	h := getBTNAuthHeader()
	h["Content-Type"] = "application/json"
	h["Content-Encoding"] = "gzip"
	return h
}

var btn_lastGetConfig int64 = 0
var btn_configureInterval = 60

var btnConfig *BTN_ConfigStruct
var btn_isGettingConfig atomic.Bool
var btn_isTaskRunning atomic.Bool

var btnRules BTN_RulesStruct
var btnExceptions BTN_ExceptionStruct
var btn_regexCache sync.Map // map[string]*regexp2.Regexp

func BTN_MatchEntry(value string, ruleRaw string) bool {
	var rule RuleEntry
	if err := json.Unmarshal([]byte(ruleRaw), &rule); err != nil {
		// 某些规则可能直接是字符串内容 (Legacy).
		return strings.Contains(value, ruleRaw)
	}

	switch strings.ToUpper(rule.Method) {
	case "EQUALS":
		return value == rule.Content
	case "STARTS_WITH":
		return strings.HasPrefix(value, rule.Content)
	case "ENDS_WITH":
		return strings.HasSuffix(value, rule.Content)
	case "CONTAINS":
		return strings.Contains(value, rule.Content)
	case "REGEX":
		var re *regexp2.Regexp
		if val, ok := btn_regexCache.Load(rule.Content); ok {
			re = val.(*regexp2.Regexp)
		} else {
			var err error
			re, err = regexp2.Compile(rule.Content, regexp2.IgnoreCase)
			if err != nil {
				Log("BTN_MatchEntry", "Invalid regex: %s", true, rule.Content)
				return false
			}
			btn_regexCache.Store(rule.Content, re)
		}
		match, _ := re.MatchString(value)
		return match
	}
	return false
}

func BTN_CheckPeer(peerIP, peerID, peerClient string, peerPort int) (bool, int, string) {
	if btnConfig == nil {
		return false, 0, ""
	}

	ipObj := net.ParseIP(peerIP)
	peerPortStr := strconv.Itoa(peerPort)

	// 1. 检查例外规则 (WhiteList).
	for _, rules := range btnExceptions.IP {
		for _, rule := range rules {
			_, subnet, err := net.ParseCIDR(rule)
			if err == nil {
				if subnet.Contains(ipObj) {
					return false, 0, ""
				}
			} else if rule == peerIP {
				return false, 0, ""
			}
		}
	}
	for _, rules := range btnExceptions.Port {
		for _, rule := range rules {
			if rule == peerPortStr || rule == "ALL" {
				return false, 0, ""
			}
		}
	}
	for _, rules := range btnExceptions.PeerID {
		for _, rule := range rules {
			if BTN_MatchEntry(peerID, rule) {
				return false, 0, ""
			}
		}
	}
	for _, rules := range btnExceptions.ClientName {
		for _, rule := range rules {
			if BTN_MatchEntry(peerClient, rule) {
				return false, 0, ""
			}
		}
	}

	// 2. 检查封禁规则 (BlockList).
	// 处理顺序: IP -> Port -> PeerID -> ClientName.
	for reason, rules := range btnRules.IP {
		for _, rule := range rules {
			_, subnet, err := net.ParseCIDR(rule)
			if err == nil {
				if subnet.Contains(ipObj) {
					return true, -1, "Bad-IP_FromBTN (" + reason + ")"
				}
			} else if rule == peerIP {
				return true, -1, "Bad-IP_FromBTN (" + reason + ")"
			}
		}
	}
	for reason, rules := range btnRules.Port {
		for _, rule := range rules {
			if rule == peerPortStr || rule == "ALL" {
				return true, peerPort, "Bad-IP_FromBTN (" + reason + ")"
			}
		}
	}
	for reason, rules := range btnRules.PeerID {
		for _, rule := range rules {
			if BTN_MatchEntry(peerID, rule) {
				return true, peerPort, "Bad-IP_FromBTN (" + reason + ")"
			}
		}
	}
	for reason, rules := range btnRules.ClientName {
		for _, rule := range rules {
			if BTN_MatchEntry(peerClient, rule) {
				return true, peerPort, "Bad-IP_FromBTN (" + reason + ")"
			}
		}
	}

	return false, 0, ""
}

func BTN_GetConfig() {
	if config.BTNConfigureURL == "" || (atomic.LoadInt64(&btn_lastGetConfig)+int64(btn_configureInterval)) > atomic.LoadInt64(&currentTimestamp) {
		return
	}
	if !btn_isGettingConfig.CompareAndSwap(false, true) {
		return
	}
	defer btn_isGettingConfig.Store(false)

	Log("Debug-BTN_GetConfig", "In progress..", false)

	atomic.StoreInt64(&btn_lastGetConfig, atomic.LoadInt64(&currentTimestamp))

	authHeader := getBTNAuthHeader()
	_, _, btnConfigContent := Fetch(config.BTNConfigureURL, false, false, false, &authHeader)
	if btnConfigContent == nil {
		Log("BTN_GetConfig", GetLangText("Error-FetchResponse"), true)
		return
	}

	// 最大 8MB.
	if len(btnConfigContent) > 8388608 {
		Log("BTN_GetConfig", GetLangText("Error-LargeFile"), true)
		return
	}

	if err := json.Unmarshal(jsonc.ToJSON(btnConfigContent), &btnConfig); err != nil {
		Log("BTN_GetConfig", GetLangText("Error-ParseConfig"), true, err.Error())
		return
	}

	// 协议版本校验 (目前我们的实现固定为 3).
	if btnConfig.MinMainVersion > 3 || btnConfig.MaxMainVersion < 3 {
		Log("BTN_GetConfig", GetLangText("Error-BTNVersionMismatch"), true, btnConfig.MinMainVersion, btnConfig.MaxMainVersion)
		btnConfig = nil
		return
	}

	Log("BTN_GetConfig", GetLangText("Success-BTNConfigLoaded"), true)
}

func BTN_SubmitPeers(torrentMap map[string]TorrentInfoStruct, currentTimestamp int64) {
	if btn_isGettingConfig.Load() || btnConfig == nil {
		return
	}

	ability, _ := btnConfig.Ability["submit_peers"]
	peers := []BTN_PeerInternalStruct{}
	torrentMapMutex.RLock()
	for torrentInfoHash, torrentInfo := range torrentMap {
		identifier := GetTorrentIdentifier(torrentInfoHash)
		for peerIP, peerInfo := range torrentInfo.Peers {
			port := 0
			for p := range peerInfo.Port {
				port = p
				break
			}
			peers = append(peers, BTN_PeerInternalStruct{
				IPAddress:          peerIP,
				PeerPort:           port,
				PeerID:             peerInfo.ID,
				ClientName:         peerInfo.Client,
				TorrentIdentifier:  identifier,
				TorrentSize:        torrentInfo.Size,
				TorrentIsPrivate:   false,
				Downloaded:         peerInfo.Downloaded,
				RTDownloadSpeed:    -1,
				Uploaded:           peerInfo.Uploaded,
				RTUploadSpeed:      -1,
				PeerProgress:       peerInfo.Progress,
				DownloaderProgress: 1.0,
				PeerFlag:           "",
			})
		}
	}
	torrentMapMutex.RUnlock()

	data := BTN_SubmitPeersStruct{
		PopulateTime: currentTimestamp * 1000,
		Peers:        peers,
	}

	submitData, err := json.Marshal(data)
	if err != nil {
		Log("BTN_SubmitPeers", GetLangText("Error-GenJSON"), true, err.Error())
		return
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(submitData); err != nil {
		Log("BTN_SubmitPeers", GetLangText("Error-Gzip"), true, err.Error())
		return
	}
	if err := gz.Close(); err != nil {
		Log("BTN_SubmitPeers", GetLangText("Error-GzipClose"), true, err.Error())
		return
	}

	authHeader := getBTNSubmitHeader()
	_, _, response := Submit(ability.Endpoint, buf.String(), false, false, &authHeader)
	if response == nil {
		Log("BTN_SubmitPeers", GetLangText("Error-FetchResponse"), true)
	}
}

func BTN_SubmitBans(blockPeerMap map[string]BlockPeerInfoStruct, currentTimestamp int64) {
	if btn_isGettingConfig.Load() || btnConfig == nil {
		return
	}

	ability, _ := btnConfig.Ability["submit_bans"]
	bans := []BTN_BanInfo{}
	blockPeerMapMutex.RLock()
	for peerIP, peerInfo := range blockPeerMap {
		identifier := GetTorrentIdentifier(peerInfo.InfoHash)
		for port := range peerInfo.Port {
			bans = append(bans, BTN_BanInfo{
				BtnBan: strings.Contains(peerInfo.Reason, "FromBTN"),
				Module: "com.SimpleTracker.qBCB." + peerInfo.Module,
				Rule:   peerInfo.Reason,
				Peer: BTN_PeerInternalStruct{
					IPAddress:          peerIP,
					PeerPort:           port,
					PeerID:             peerInfo.ID,
					ClientName:         peerInfo.Client,
					TorrentIdentifier:  identifier,
					TorrentSize:        0, // 封禁列表中不记录种子大小.
					TorrentIsPrivate:   false,
					Downloaded:         peerInfo.Downloaded,
					RTDownloadSpeed:    -1,
					Uploaded:           peerInfo.Uploaded,
					RTUploadSpeed:      -1,
					PeerProgress:       0,
					DownloaderProgress: 1.0,
					PeerFlag:           "",
				},
				BanAt:       peerInfo.Timestamp * 1000,
				BanUniqueID: peerIP + ":" + strconv.Itoa(port),
			})
		}
	}
	blockPeerMapMutex.RUnlock()

	data := BTN_SubmitBansStruct{
		PopulateTime: currentTimestamp * 1000,
		Bans:         bans,
	}

	submitData, err := json.Marshal(data)
	if err != nil {
		Log("BTN_SubmitBans", GetLangText("Error-GenJSON"), true, err.Error())
		return
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(submitData); err != nil {
		Log("BTN_SubmitBans", GetLangText("Error-Gzip"), true, err.Error())
		return
	}
	if err := gz.Close(); err != nil {
		Log("BTN_SubmitBans", GetLangText("Error-GzipClose"), true, err.Error())
		return
	}

	authHeader := getBTNSubmitHeader()
	_, _, response := Submit(ability.Endpoint, buf.String(), false, false, &authHeader)
	if response == nil {
		Log("BTN_SubmitBans", GetLangText("Error-FetchResponse"), true)
	}
}

func BTN_SubmitHistories(torrentMap map[string]TorrentInfoStruct, lastTorrentMap map[string]TorrentInfoStruct, currentTimestamp int64) {
	if btn_isGettingConfig.Load() || btnConfig == nil {
		return
	}

	ability, _ := btnConfig.Ability["submit_histories"]
	peers := []BTN_PeerHistoryStruct{}
	torrentMapMutex.RLock()
	lastTorrentMapMutex.RLock()
	for torrentInfoHash, torrentInfo := range torrentMap {
		identifier := GetTorrentIdentifier(torrentInfoHash)
		lastTorrentInfo, hasLastTorrent := lastTorrentMap[torrentInfoHash]

		for peerIP, peerInfo := range torrentInfo.Peers {
			port := 0
			for p := range peerInfo.Port {
				port = p
				break
			}

			var dlOffset, upOffset int64 = 0, 0
			if hasLastTorrent {
				if lastPeerInfo, hasLastPeer := lastTorrentInfo.Peers[peerIP]; hasLastPeer {
					if peerInfo.Downloaded >= lastPeerInfo.Downloaded {
						dlOffset = peerInfo.Downloaded - lastPeerInfo.Downloaded
					} else {
						dlOffset = peerInfo.Downloaded
					}
					if peerInfo.Uploaded >= lastPeerInfo.Uploaded {
						upOffset = peerInfo.Uploaded - lastPeerInfo.Uploaded
					} else {
						upOffset = peerInfo.Uploaded
					}
				}
			}

			peers = append(peers, BTN_PeerHistoryStruct{
				BTN_PeerInternalStruct: BTN_PeerInternalStruct{
					IPAddress:          peerIP,
					PeerPort:           port,
					PeerID:             peerInfo.ID,
					ClientName:         peerInfo.Client,
					TorrentIdentifier:  identifier,
					TorrentSize:        torrentInfo.Size,
					TorrentIsPrivate:   false,
					Downloaded:         peerInfo.Downloaded,
					RTDownloadSpeed:    -1,
					Uploaded:           peerInfo.Uploaded,
					RTUploadSpeed:      -1,
					PeerProgress:       peerInfo.Progress,
					DownloaderProgress: 1.0,
					PeerFlag:           "",
				},
				DownloadedOffset: dlOffset,
				UploadedOffset:   upOffset,
				FirstTimeSeen:    currentTimestamp * 1000,
				LastTimeSeen:     currentTimestamp * 1000,
			})
		}
	}
	lastTorrentMapMutex.RUnlock()
	torrentMapMutex.RUnlock()

	data := BTN_SubmitHistoriesStruct{
		PopulateTime: currentTimestamp * 1000,
		Peers:        peers,
	}

	submitData, err := json.Marshal(data)
	if err != nil {
		Log("BTN_SubmitHistories", GetLangText("Error-GenJSON"), true, err.Error())
		return
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(submitData); err != nil {
		Log("BTN_SubmitHistories", GetLangText("Error-Gzip"), true, err.Error())
		return
	}
	if err := gz.Close(); err != nil {
		Log("BTN_SubmitHistories", GetLangText("Error-GzipClose"), true, err.Error())
		return
	}

	authHeader := getBTNSubmitHeader()
	_, _, response := Submit(ability.Endpoint, buf.String(), false, false, &authHeader)
	if response == nil {
		Log("BTN_SubmitHistories", GetLangText("Error-FetchResponse"), true)
	}
}

func BTN_Reconfigure() {
	if btn_isGettingConfig.Load() || btnConfig == nil {
		return
	}

	ability, _ := btnConfig.Ability["reconfigure"]
	authHeader := getBTNAuthHeader()
	statusCode, _, response := Fetch(ability.Endpoint+"?rev="+btnConfig.Ability["reconfigure"].Version, false, false, false, &authHeader)
	if response == nil {
		if statusCode == 204 {
			Log("BTN_Reconfigure", GetLangText("Debug-BTNConfigNoChange"), false)
		} else {
			Log("BTN_Reconfigure", GetLangText("Error-FetchResponse"), true)
		}
		return
	}

	BTN_GetConfig()
}

func BTN_Rules() {
	if btn_isGettingConfig.Load() || btnConfig == nil {
		return
	}

	ability, _ := btnConfig.Ability["rules"]
	authHeader := getBTNAuthHeader()
	rulesEndpoint := ability.Endpoint
	if btnRules.Version != "" {
		if strings.Contains(rulesEndpoint, "?") {
			rulesEndpoint += "&rev=" + btnRules.Version
		} else {
			rulesEndpoint += "?rev=" + btnRules.Version
		}
	}

	statusCode, _, response := Fetch(rulesEndpoint, false, false, false, &authHeader)
	if response == nil {
		if statusCode == 204 {
			Log("BTN_Rules", GetLangText("Debug-BTNNoChange"), false)
		} else {
			Log("BTN_Rules", GetLangText("Error-FetchResponse"), true)
		}
		return
	}

	// 处理规则数据.
	if err := json.Unmarshal(response, &btnRules); err != nil {
		Log("BTN_Rules", GetLangText("Error-Parse"), true, err.Error())
		return
	}

	Log("BTN_Rules", GetLangText("Success-BTNRegLoaded"), true, btnRules.Version)
}

func BTN_Exception() {
	if btn_isGettingConfig.Load() || btnConfig == nil {
		return
	}

	ability, _ := btnConfig.Ability["exception"]
	authHeader := getBTNAuthHeader()
	exceptionEndpoint := ability.Endpoint
	if btnExceptions.Version != "" {
		if strings.Contains(exceptionEndpoint, "?") {
			exceptionEndpoint += "&rev=" + btnExceptions.Version
		} else {
			exceptionEndpoint += "?rev=" + btnExceptions.Version
		}
	}

	statusCode, _, response := Fetch(exceptionEndpoint, false, false, false, &authHeader)
	if response == nil {
		if statusCode == 204 {
			Log("BTN_Exception", GetLangText("Debug-BTNNoChange"), false)
		} else {
			Log("BTN_Exception", GetLangText("Error-FetchResponse"), true)
		}
		return
	}

	// 处理例外规则数据.
	if err := json.Unmarshal(response, &btnExceptions); err != nil {
		Log("BTN_Exception", GetLangText("Error-Parse"), true, err.Error())
		return
	}

	Log("BTN_Exception", GetLangText("Success-BTNExceptionLoaded"), true, btnExceptions.Version)
}

func BTN_Task() {
	if btn_isGettingConfig.Load() || btn_isTaskRunning.Load() {
		return
	}

	if btn_isTaskRunning.CompareAndSwap(false, true) {
		ts := atomic.LoadInt64(&currentTimestamp)
		ms := ts * 1000
		GoWithCrashLog("BTN_Task", func() {
			defer btn_isTaskRunning.Store(false)

			btn_taskMutex.Lock()
			defer btn_taskMutex.Unlock()

			executeTask := func(name string, taskFunc func()) {
				ability, exists := btnConfig.Ability[name]
				if !exists {
					return
				}

				// 处理随机初始化延迟 (一次性).
				initDelay, applied := btn_taskInitialDelay[name]
				if !applied {
					if ability.RandomInitialDelay > 0 {
						initDelay = int64(rand.Intn(int(ability.RandomInitialDelay)))
					}
					btn_taskInitialDelay[name] = initDelay
				}
				if ms < programStartTimestamp*1000+initDelay {
					return
				}

				// 处理任务间隔.
				lastExec := btn_lastTaskExecution[name]
				if ms >= lastExec+int64(ability.Interval) {
					taskFunc()
					btn_lastTaskExecution[name] = ms
				}
			}

			if config.BTNSubmitPeers {
				executeTask("submit_peers", func() { BTN_SubmitPeers(torrentMap, ts) })
			}
			if config.BTNSubmitBans {
				executeTask("submit_bans", func() { BTN_SubmitBans(blockPeerMap, ts) })
			}
			if config.BTNSubmitHistories {
				executeTask("submit_histories", func() { BTN_SubmitHistories(torrentMap, lastTorrentMap, ts) })
			}
			if config.BTNSyncRules {
				executeTask("rules", func() { BTN_Rules() })
			}
			if config.BTNSyncException {
				executeTask("exception", func() { BTN_Exception() })
			}
			executeTask("reconfigure", func() { BTN_Reconfigure() })
		})
	}
}

// GetTorrentIdentifier 生成种子不可逆匿名识别符.
func GetTorrentIdentifier(torrentInfoHash string) string {
	// 将 torrentInfoHash 转为小写.
	torrentInfoHandled := strings.ToLower(torrentInfoHash)

	// 使用 CRC32 生成盐值.
	salt := crc32.ChecksumIEEE([]byte(torrentInfoHandled))

	// 生成大端序盐值字符串 (8 位十六进制).
	saltStr := fmt.Sprintf("%08x", salt)

	// 将 torrentInfoHandled 和 saltStr 连接起来.
	combined := torrentInfoHandled + saltStr

	// 使用 SHA256 进行哈希.
	hash := sha256.New()
	hash.Write([]byte(combined))
	hashBytes := hash.Sum(nil)

	// 返回 SHA256 哈希值的小写十六进制表示.
	return hex.EncodeToString(hashBytes)
}
