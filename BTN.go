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
	"time"

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

type BTN_PeerStruct struct {
	TorrentIdentifier     string  `json:"torrent_identifier"`
	TorrentIsPrivate      bool    `json:"torrent_is_private"`
	TorrentSize           int64   `json:"torrent_size"`
	Downloader            string  `json:"downloader"` // 我们作为下载器的标识.
	DownloaderProgress    float64 `json:"downloader_progress"`
	PeerIP                string  `json:"peer_ip"`
	PeerPort              int     `json:"peer_port"`
	PeerID                string  `json:"peer_id"`
	ClientName            string  `json:"peer_client_name"`
	PeerProgress          float64 `json:"peer_progress"`
	ToPeerTraffic         int64   `json:"to_peer_traffic"` // 上传量.
	ToPeerTrafficOffset   int64   `json:"to_peer_traffic_offset"`
	FromPeerTraffic       int64   `json:"from_peer_traffic"` // 下载量.
	FromPeerTrafficOffset int64   `json:"from_peer_traffic_offset"`
	FirstTimeSeen         int64   `json:"first_time_seen"` // 秒级时间戳 (int64 代替 Timestamp).
	LastTimeSeen          int64   `json:"last_time_seen"`
	PeerFlag              string  `json:"peer_last_flags"`
}

type BTN_SubmitPeersStruct struct {
	PopulateTime int64            `json:"populate_time"`
	Peers        []BTN_PeerStruct `json:"peers"`
}

type BTN_SubmitBansStruct struct {
	PopulateTime int64         `json:"populate_time"`
	Bans         []BTN_BanInfo `json:"bans"`
}

type BTN_BanInfo struct {
	BanAt              int64   `json:"ban_at"`
	PeerIP             string  `json:"peer_ip"`
	PeerPort           int     `json:"peer_port"`
	PeerID             string  `json:"peer_id"`
	PeerClientName     string  `json:"peer_client_name"`
	PeerProgress       float64 `json:"peer_progress"`
	PeerFlag           string  `json:"peer_flag"`
	TorrentIdentifier  string  `json:"torrent_identifier"`
	TorrentIsPrivate   bool    `json:"torrent_is_private"`
	TorrentSize        int64   `json:"torrent_size"`
	FromPeerTraffic    int64   `json:"from_peer_traffic"`
	ToPeerTraffic      int64   `json:"to_peer_traffic"`
	DownloaderProgress float64 `json:"downloader_progress"`
	Module             string  `json:"module"`
	Rule               string  `json:"rule"`
	Description        string  `json:"description"`
	StructuredData     string  `json:"structured_data"`
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

var btnProtocol = "BTN-Protocol/3.0.0"
var btnUserAgent = programUserAgent + " " + btnProtocol

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

	Log("BTN_GetConfig", "BTN configuration loaded successfully", true)
}

func BTN_SubmitPeers(torrentMap map[string]TorrentInfoStruct, currentTimestamp int64) {
	if btn_isGettingConfig.Load() || btnConfig == nil {
		return
	}

	ability, exists := btnConfig.Ability["submit_peers"]
	if !exists {
		return
	}

	delay := time.Duration(ability.Interval) * time.Millisecond
	if ability.RandomInitialDelay > 0 {
		delay += time.Duration(rand.Intn(int(ability.RandomInitialDelay))) * time.Millisecond
	}

	time.Sleep(delay)

	peers := []BTN_PeerStruct{}
	torrentMapMutex.RLock()
	for torrentInfoHash, torrentInfo := range torrentMap {
		identifier := GetTorrentIdentifier(torrentInfoHash)
		for peerIP, peerInfo := range torrentInfo.Peers {
			peers = append(peers, BTN_PeerStruct{
				TorrentIdentifier:     identifier,
				TorrentIsPrivate:      false, // 默认值.
				TorrentSize:           torrentInfo.Size,
				Downloader:            "qBCB",
				DownloaderProgress:    1.0,
				PeerIP:                peerIP,
				PeerPort:              -1,
				PeerID:                "",
				ClientName:            "",
				PeerProgress:          peerInfo.Progress,
				ToPeerTraffic:         peerInfo.Uploaded,
				ToPeerTrafficOffset:   0,
				FromPeerTraffic:       -1,
				FromPeerTrafficOffset: 0,
				FirstTimeSeen:         currentTimestamp,
				LastTimeSeen:          currentTimestamp,
				PeerFlag:              "",
			})
		}
	}
	torrentMapMutex.RUnlock()

	data := BTN_SubmitPeersStruct{
		PopulateTime: currentTimestamp,
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

	ability, exists := btnConfig.Ability["submit_bans"]
	if !exists {
		return
	}

	delay := time.Duration(ability.Interval) * time.Millisecond
	if ability.RandomInitialDelay > 0 {
		delay += time.Duration(rand.Intn(int(ability.RandomInitialDelay))) * time.Millisecond
	}

	time.Sleep(delay)

	bans := []BTN_BanInfo{}
	blockPeerMapMutex.RLock()
	for peerIP, peerInfo := range blockPeerMap {
		identifier := GetTorrentIdentifier(peerInfo.InfoHash)
		// 检查 BlockPeerInfoStruct 定义.
		for port := range peerInfo.Port {
			bans = append(bans, BTN_BanInfo{
				BanAt:              currentTimestamp * 1000, // 规范通常使用毫秒.
				PeerIP:             peerIP,
				PeerPort:           port,
				PeerID:             "",
				PeerClientName:     "",
				PeerProgress:       0,
				PeerFlag:           "",
				TorrentIdentifier:  identifier,
				TorrentIsPrivate:   false,
				TorrentSize:        0,
				FromPeerTraffic:    -1,
				ToPeerTraffic:      0,
				DownloaderProgress: 1.0,
				Module:             "com.SimpleTracker.qBCB." + peerInfo.Module,
				Rule:               peerInfo.Reason,
				Description:        "Banned by qBCB",
				StructuredData:     "",
			})
		}
	}
	blockPeerMapMutex.RUnlock()

	data := BTN_SubmitBansStruct{
		PopulateTime: currentTimestamp,
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

func BTN_Reconfigure() {
	if btn_isGettingConfig.Load() || btnConfig == nil {
		return
	}

	ability, exists := btnConfig.Ability["reconfigure"]
	if !exists {
		return
	}

	delay := time.Duration(ability.Interval) * time.Millisecond
	if ability.RandomInitialDelay > 0 {
		delay += time.Duration(rand.Intn(int(ability.RandomInitialDelay))) * time.Millisecond
	}

	time.Sleep(delay)

	authHeader := getBTNAuthHeader()
	statusCode, _, response := Fetch(ability.Endpoint+"?rev="+btnConfig.Ability["reconfigure"].Version, false, false, false, &authHeader)
	if response == nil {
		if statusCode == 204 {
			Log("BTN_Reconfigure", "Configuration has not changed", true)
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

	ability, exists := btnConfig.Ability["rules"]
	if !exists {
		return
	}

	delay := time.Duration(ability.Interval) * time.Millisecond
	if ability.RandomInitialDelay > 0 {
		delay += time.Duration(rand.Intn(int(ability.RandomInitialDelay))) * time.Millisecond
	}

	time.Sleep(delay)

	authHeader := getBTNAuthHeader()
	statusCode, _, response := Fetch(ability.Endpoint+"?rev="+btnRules.Version, false, false, false, &authHeader)
	if response == nil {
		if statusCode == 204 {
			Log("BTN_Rules", "Rules have not changed", true)
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

	// 保存规则数据以供使用.
	Log("BTN_Rules", "Rules loaded successfully: %v", true, btnRules)
}

func BTN_Exception() {
	if btn_isGettingConfig.Load() || btnConfig == nil {
		return
	}

	ability, exists := btnConfig.Ability["exception"]
	if !exists {
		return
	}

	delay := time.Duration(ability.Interval) * time.Millisecond
	if ability.RandomInitialDelay > 0 {
		delay += time.Duration(rand.Intn(int(ability.RandomInitialDelay))) * time.Millisecond
	}

	time.Sleep(delay)

	authHeader := getBTNAuthHeader()
	statusCode, _, response := Fetch(ability.Endpoint+"?rev="+btnExceptions.Version, false, false, false, &authHeader)
	if response == nil {
		if statusCode == 204 {
			Log("BTN_Exception", "Exceptions have not changed", true)
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

	// 保存例外规则数据以供使用.
	Log("BTN_Exception", "Exceptions loaded successfully: %v", true, btnExceptions)
}

func BTN_Task() {
	if btn_isGettingConfig.Load() || btn_isTaskRunning.Load() {
		return
	}

	if btn_isTaskRunning.CompareAndSwap(false, true) {
		ts := atomic.LoadInt64(&currentTimestamp)
		GoWithCrashLog("BTN_Task", func() {
			defer btn_isTaskRunning.Store(false)
			BTN_SubmitPeers(torrentMap, ts)
			BTN_SubmitBans(blockPeerMap, ts)
			BTN_Reconfigure()
			BTN_Rules()
			BTN_Exception()
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
