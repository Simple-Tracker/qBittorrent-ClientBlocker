package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash/crc32"
	"math/rand"
	"strings"
	"time"
	"github.com/tidwall/jsonc"
)

type BTN_Ability struct {
	Interval          uint32 `json:"interval"`
	Endpoint          string `json:"endpoint"`
	RandomInitialDelay uint32 `json:"random_initial_delay"`
	Version           string `json:"version"`
}

type BTN_ConfigStruct struct {
	MinMainVersion uint32                 `json:"min_protocol_version"`
	MaxMainVersion uint32                 `json:"max_protocol_version"`
	Ability        map[string]BTN_Ability `json:"ability"`
}

type BTN_PeerStruct struct {
	IPAddress         string  `json:"ip_address"`
	PeerPort          int     `json:"peer_port"`
	PeerID            string  `json:"peer_id"`
	ClientName        string  `json:"client_name"`
	TorrentIdentifier string  `json:"torrent_identifier"`
	TorrentSize       int64   `json:"torrent_size"`
	Downloaded        int64   `json:"downloaded"`
	RTDownloadSpeed   int64   `json:"rt_download_speed"`
	Uploaded          int64   `json:"uploaded"`
	RTUploadSpeed     int64   `json:"rt_upload_speed"`
	PeerProgress      float64 `json:"peer_progress"`
	DownloaderProgress float64 `json:"downloader_progress"`
	PeerFlag          string  `json:"peer_flag"`
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
	BTNBan    bool           `json:"btn_ban"`
	Module    string         `json:"module"`
	Rule      string         `json:"rule"`
	Peer      BTN_PeerStruct `json:"peer"`
}

type BTN_RulesStruct struct {
	Version   string                 `json:"version"`
	PeerID    map[string][]RuleEntry `json:"peer_id"`
	ClientName map[string][]RuleEntry `json:"client_name"`
	IP        map[string][]string    `json:"ip"`
	Port      map[string][]string    `json:"port"`
}

type RuleEntry struct {
	Method  string `json:"method"`
	Content string `json:"content"`
}

type BTN_ExceptionStruct struct {
	Version   string                 `json:"version"`
	PeerID    map[string][]RuleEntry `json:"peer_id"`
	ClientName map[string][]RuleEntry `json:"client_name"`
	IP        map[string][]string    `json:"ip"`
	Port      map[string][]string    `json:"port"`
}

var btnProtocol = "BTN-Protocol/0.0.0-dev"
var btnUserAgent = programUserAgent + " " + btnProtocol
var btnHeader = map[string]string{
	"User-Agent": btnUserAgent,
}

var btn_lastGetConfig int64 = 0
var btn_configureInterval = 60

var btnConfig *BTN_ConfigStruct
var btn_isGettingConfig bool = false

var btnRules BTN_RulesStruct
var btnExceptions BTN_ExceptionStruct

func BTN_GetConfig() {
	if config.BTNConfigureURL == "" || (btn_lastGetConfig+int64(btn_configureInterval)) > currentTimestamp {
		return
	}

	Log("Debug-BTN_GetConfig", "In progress..", false)
	btn_isGettingConfig = true

	btn_lastGetConfig = currentTimestamp

	_, _, btnConfigContent := Fetch(config.BTNConfigureURL, false, false, false, &btnHeader)
	if btnConfigContent == nil {
		Log("BTN_GetConfig", GetLangText("Error-FetchResponse"), true)
		btn_isGettingConfig = false
		return
	}

	// Max 8MB.
	if len(btnConfigContent) > 8388608 {
		Log("BTN_GetConfig", GetLangText("Error-LargeFile"), true)
		btn_isGettingConfig = false
		return
	}

	if err := json.Unmarshal(jsonc.ToJSON(btnConfigContent), &btnConfig); err != nil {
		Log("BTN_GetConfig", GetLangText("Error-ParseConfig"), true, err.Error())
		btn_isGettingConfig = false
		return
	}

	Log("BTN_GetConfig", "BTN configuration loaded successfully", true)
	btn_isGettingConfig = false
}

func BTN_SubmitPeers(torrentMap map[string]TorrentInfoStruct, currentTimestamp int64) {
	if btn_isGettingConfig || btnConfig == nil {
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
	for torrentInfoHash, torrentInfo := range torrentMap {
		for peerIP, peerInfo := range torrentInfo.Peers {
			torrentIdentifier := GetTorrentIdentifier(torrentInfoHash)
			peers = append(peers, BTN_PeerStruct{
				IPAddress:         peerIP,
				PeerPort:          -1, // 默认端口号
				PeerID:            "", // 默认 PeerID
				ClientName:        "", // 默认 ClientName
				TorrentIdentifier: torrentIdentifier,
				TorrentSize:       torrentInfo.Size,
				Downloaded:        -1, // 默认下载量
				RTDownloadSpeed:   -1, // 默认实时下载速度
				Uploaded:          peerInfo.Uploaded,
				RTUploadSpeed:     -1, // 默认实时上传速度
				PeerProgress:      peerInfo.Progress,
				DownloaderProgress: 1.0, // 默认下载进度
				PeerFlag:          "", // 默认标志
			})
		}
	}

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

	_, _, response := Submit(ability.Endpoint, buf.String(), false, false, &btnHeader)
	if response == nil {
		Log("BTN_SubmitPeers", GetLangText("Error-FetchResponse"), true)
	}
}

/* 
注释掉 BTN_SubmitBans 方法，原因：信息缺失严重，无法根据协议规范完整实现

func BTN_SubmitBans(blockPeerMap map[string]BlockPeerInfoStruct, currentTimestamp int64) {
	if btn_isGettingConfig || btnConfig == nil {
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
	for peerIP, peerInfo := range blockPeerMap {
		for port := range peerInfo.Port {
			torrentIdentifier := GetTorrentIdentifier(peerInfo.InfoHash)
			bans = append(bans, BTN_BanInfo{
				BTNBan: false,
				Module: "com.SimpleTracker.qBCB." + peerInfo.Module, // 使用 BlockPeerInfoStruct 的 Module
				Rule:   peerInfo.Reason, // 使用 BlockPeerInfoStruct 的 Reason
				Peer: BTN_PeerStruct{
					IPAddress:         peerIP,
					PeerPort:          port,
					PeerID:            "", // 默认 PeerID
					ClientName:        "", // 默认 ClientName
					TorrentIdentifier: torrentIdentifier,
					TorrentSize:       0, // 默认种子大小
					Downloaded:        -1, // 默认下载量
					RTDownloadSpeed:   -1, // 默认实时下载速度
					Uploaded:          0, // 默认上传量
					RTUploadSpeed:     -1, // 默认实时上传速度
					PeerProgress:      0.0, // 默认进度
					DownloaderProgress: 1.0, // 默认下载进度
					PeerFlag:          "", // 默认标志
				},
			})
		}
	}

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

	_, _, response := Submit(ability.Endpoint, buf.String(), false, false, &btnHeader)
	if response == nil {
		Log("BTN_SubmitBans", GetLangText("Error-FetchResponse"), true)
	}
}
*/

func BTN_Reconfigure() {
	if btn_isGettingConfig || btnConfig == nil {
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

	statusCode, _ , response := Fetch(ability.Endpoint+"?rev="+btnConfig.Ability["reconfigure"].Version, false, false, false, &btnHeader)
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
	if btn_isGettingConfig || btnConfig == nil {
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

	statusCode, _ , response := Fetch(ability.Endpoint+"?rev="+btnRules.Version, false, false, false, &btnHeader)
	if response == nil {
		if statusCode == 204 {
			Log("BTN_Rules", "Rules have not changed", true)
		} else {
			Log("BTN_Rules", GetLangText("Error-FetchResponse"), true)
		}
		return
	}

	// 处理规则数据
	if err := json.Unmarshal(response, &btnRules); err != nil {
		Log("BTN_Rules", GetLangText("Error-Parse"), true, err.Error())
		return
	}

	// 保存规则数据以供使用
	Log("BTN_Rules", "Rules loaded successfully: %v", true, btnRules)
}

func BTN_Exception() {
	if btn_isGettingConfig || btnConfig == nil {
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

	statusCode, _ , response := Fetch(ability.Endpoint+"?rev="+btnExceptions.Version, false, false, false, &btnHeader)
	if response == nil {
		if statusCode == 204 {
			Log("BTN_Exception", "Exceptions have not changed", true)
		} else {
			Log("BTN_Exception", GetLangText("Error-FetchResponse"), true)
		}
		return
	}

	// 处理例外规则数据
	if err := json.Unmarshal(response, &btnExceptions); err != nil {
		Log("BTN_Exception", GetLangText("Error-Parse"), true, err.Error())
		return
	}

	// 保存例外规则数据以供使用
	Log("BTN_Exception", "Exceptions loaded successfully: %v", true, btnExceptions)
}

func BTN_Task() {
	if btn_isGettingConfig {
		return
	}
	go BTN_SubmitPeers(torrentMap, currentTimestamp)
	// 去除 BTN_SubmitBans 方法调用，原因：信息缺失严重，无法根据协议规范完整实现
	// go BTN_SubmitBans(blockPeerMap, currentTimestamp)
	go BTN_Reconfigure()
	go BTN_Rules()
	go BTN_Exception()
}

// flipBytes 进行端序翻转
func flipBytes(a []byte) []byte {
	b := make([]byte, len(a))
	for i := 0; i < len(a); i++ {
		b[i] = a[len(a)-1-i]
	}
	return b
}

// GetTorrentIdentifier 生成种子不可逆匿名识别符
func GetTorrentIdentifier(torrentInfoHash string) string {
	// 将 torrentInfoHash 转为小写
	torrentInfoHandled := strings.ToLower(torrentInfoHash)

	// 使用 CRC32 生成盐值
	salt := crc32.ChecksumIEEE([]byte(torrentInfoHandled))

	// 将盐值转为字节数组
	saltBytes := []byte{byte(salt >> 24), byte(salt >> 16), byte(salt >> 8), byte(salt)}

	// 进行端序翻转
	saltFlipped := flipBytes(saltBytes)

	// 将翻转后的盐值转为字符串
	saltStr := hex.EncodeToString(saltFlipped)

	// 将 torrentInfoHandled 和 saltStr 连接起来
	combined := torrentInfoHandled + saltStr

	// 使用 SHA256 进行哈希
	hash := sha256.New()
	hash.Write([]byte(combined))
	hashBytes := hash.Sum(nil)

	// 返回 SHA256 哈希值的十六进制表示
	return hex.EncodeToString(hashBytes)
}