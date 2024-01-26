package main

import (
	"os"
	"fmt"
	"flag"
	"time"
	"regexp"
	"reflect"
	"strings"
	"strconv"
	"net"
	"net/url"
	"net/http"
	"net/http/cookiejar"
	"io/ioutil"
	"encoding/json"
)

type IPInfoStruct struct {
	TorrentUploaded map[string]int64
}
type PeerInfoStruct struct {
	Timestamp int64
	Port      map[int]bool
	Progress  float64
	Uploaded  int64
}
type BlockPeerInfoStruct struct {
	Timestamp int64
	Port      int
}
type MainDataStruct struct {
	FullUpdate bool                     `json:"full_update"`
	Torrents   map[string]TorrentStruct `json:"torrents"`
}
type TorrentStruct struct {
	NumLeechs int64 `json:"num_leechs"`
	TotalSize int64 `json:"total_size"`
}
type PeerStruct struct {
	IP       string
	Port     int
	Client   string
	Progress float64
	Uploaded int64
}
type TorrentPeersStruct struct {
	FullUpdate bool                  `json:"full_update"`
	Peers      map[string]PeerStruct `json:"peers"`
}
type ConfigStruct struct {
	Debug                         bool
	Interval                      uint32
	CleanInterval                 uint32
	PeerMapCleanInterval          uint32
	BanTime                       uint32
	SleepTime                     uint32
	Timeout                       uint32
	IPUploadedCheck               bool
	IPUpCheckInterval             uint32
	IPUpCheckIncrementMB          uint32
	MaxIPPortCount                uint32
	BanByProgressUploaded         bool
	BanByPUStartMB                uint32
	BanByPUStartPrecent           uint32
	BanByPUAntiErrorRatio         uint32
	BanByRelativeProgressUploaded bool
	BanByRelativePUStartMB        uint32
	BanByRelativePUStartPrecent   uint32
	BanByRelativePUAntiErrorRatio uint32
	LongConnection                bool
	LogToFile                     bool
	LogDebug                      bool
	QBURL                         string
	QBUsername                    string
	QBPassword                    string
	BlockList                     []string
}

var useNewBanPeersMethod = false
var todayStr = ""
var currentTimestamp int64 = 0
var lastCleanTimestamp int64 = 0
var lastIPCleanTimestamp int64 = 0
var lastPeerCleanTimestamp int64 = 0
var ipMap = make(map[string]IPInfoStruct)
var peerMap = make(map[string]PeerInfoStruct)
var blockPeerMap = make(map[string]BlockPeerInfoStruct)
var blockListCompiled []*regexp.Regexp
var cookieJar, _ = cookiejar.New(nil)
var httpTransport = &http.Transport {
	DisableKeepAlives:   false,
	ForceAttemptHTTP2:   false,
	MaxConnsPerHost:     32,
	MaxIdleConns:        32,
	MaxIdleConnsPerHost: 32,
}
var httpClient = http.Client {
	Timeout:   6 * time.Second,
	Jar:       cookieJar,
	Transport: httpTransport,
}
var config = ConfigStruct {
	Debug:                         false,
	Interval:                      2,
	CleanInterval:                 3600,
	PeerMapCleanInterval:          60,
	BanTime:                       86400,
	SleepTime:                     20,
	Timeout:                       6,
	IPUploadedCheck:               false,
	IPUpCheckInterval:             3600,
	IPUpCheckIncrementMB:          180000,
	MaxIPPortCount:                0,
	BanByProgressUploaded:         false,
	BanByPUStartMB:                10,
	BanByPUStartPrecent:           2,
	BanByPUAntiErrorRatio:         5,
	BanByRelativeProgressUploaded: false,
	BanByRelativePUStartMB:        10,
	BanByRelativePUStartPrecent:   2,
	BanByRelativePUAntiErrorRatio: 5,
	LongConnection:                true,
	LogToFile:                     true,
	LogDebug:                      false,
	QBURL:                         "http://127.0.0.1:990",
	QBUsername:                    "",
	QBPassword:                    "",
	BlockList:                     []string {},
}
var configFilename string
var configLastMod int64 = 0
var logFile *os.File

func GetDateTime(withTime bool) string {
	formatStr := "2006-01-02"
	if withTime {
		formatStr += " 15:04:05"
	}
	return time.Now().Format(formatStr)
}
func Log(module string, str string, logToFile bool, args ...interface {}) {
	if strings.HasPrefix(module, "Debug") {
		if !config.Debug {
			return
		} else if config.LogDebug {
			logToFile = true
		}
	}
	logStr := fmt.Sprintf("[" + GetDateTime(true) + "][" + module + "] " + str + ".\n", args...)
	if config.LogToFile && logToFile && logFile != nil {
		if _, err := logFile.Write([]byte(logStr)); err != nil {
			Log("Log", "写入日志时发生了错误: %s", false, err.Error())
		}
	}
	fmt.Print(logStr)
}
func LoadLog() {
	tmpTodayStr := GetDateTime(false)
	if todayStr != tmpTodayStr {
		todayStr = tmpTodayStr
		logFile.Close()

		tLogFile, err := os.OpenFile("logs/" + todayStr + ".txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			tLogFile.Close()
			tLogFile = nil
			Log("LoadLog", "访问日志时发生了错误: %s", false, err.Error())
		}
		logFile = tLogFile
	}
}
func LoadConfig() bool {
	configFileStat, err := os.Stat(configFilename)
	if err != nil {
		Log("Debug-LoadConfig", "读取配置文件元数据时发生了错误: %s", false, err.Error())
		return false
	}
	tmpConfigLastMod := configFileStat.ModTime().Unix()
	if tmpConfigLastMod <= configLastMod {
		return true
	}
	if configLastMod != 0 {
		Log("Debug-LoadConfig", "发现配置文件更改, 正在进行热重载", false)
	}
	configFile, err := ioutil.ReadFile(configFilename)
	if err != nil {
		Log("LoadConfig", "读取配置文件时发生了错误: %s", false, err.Error())
		return false
	}
	configLastMod = tmpConfigLastMod
	if err := json.Unmarshal(configFile, &config); err != nil {
		Log("LoadConfig", "解析配置文件时发生了错误: %s", false, err.Error())
		return false
	}
	Log("LoadConfig", "读取配置文件成功", true)
	InitConfig()
	return true
}
func InitConfig() {
	if config.LogToFile {
		os.Mkdir("logs", os.ModePerm)
		LoadLog()
	} else if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	if config.Interval < 1 {
		config.Interval = 1
	}
	if config.Timeout < 1 {
		config.Timeout = 1
	}
	if !config.LongConnection {
		httpClient = http.Client {
			Timeout:   time.Duration(config.Timeout) * time.Second,
			Jar:       cookieJar,
		}
	} else if config.Timeout != 6 {
		httpClient = http.Client {
			Timeout:   time.Duration(config.Timeout) * time.Second,
			Jar:       cookieJar,
			Transport: httpTransport,
		}
	}
	t := reflect.TypeOf(config)
	v := reflect.ValueOf(config)
	for k := 0; k < t.NumField(); k++ {
		Log("LoadConfig-Current", "%v: %v", true, t.Field(k).Name, v.Field(k).Interface())
	}
	blockListCompiled = make([]*regexp.Regexp, len(config.BlockList))
	for k, v := range config.BlockList {
		Log("Debug-LoadConfig-CompileBlockList", "%s", false, v)
		reg, err := regexp.Compile("(?i)" + v)
		if err != nil {
			Log("LoadConfig-CompileBlockList", "表达式 %s 有错误", true, v)
			continue
		}
		blockListCompiled[k] = reg
	}
}
func CheckPrivateIP(ip string) bool {
	ipParsed := net.ParseIP(ip)
	return ipParsed.IsPrivate()
}
func AddIPInfo(clientIP string, torrentInfoHash string, clientUploaded int64) {
	if !config.IPUploadedCheck {
		return
	}
	var clientTorrentUploadedMap map[string]int64
	if info, exist := ipMap[clientIP]; !exist {
		clientTorrentUploadedMap = make(map[string]int64)
	} else {
		clientTorrentUploadedMap = info.TorrentUploaded
	}
	clientTorrentUploadedMap[torrentInfoHash] = clientUploaded
	ipMap[clientIP] = IPInfoStruct { TorrentUploaded: clientTorrentUploadedMap }
}
func AddPeerInfo(peerIP string, peerPort int, peerProgress float64, peerUploaded int64) {
	if config.MaxIPPortCount <= 0 && !config.BanByRelativeProgressUploaded {
		return
	}
	peerIP = strings.ToLower(peerIP)
	var peerPortMap map[int]bool
	if peer, exist := peerMap[peerIP]; !exist {
		peerPortMap = make(map[int]bool)
	} else {
		peerPortMap = peer.Port
	}
	peerPortMap[peerPort] = true
	peerMap[peerIP] = PeerInfoStruct { Timestamp: currentTimestamp, Port: peerPortMap, Progress: peerProgress, Uploaded: peerUploaded }
}
func AddBlockPeer(peerIP string, peerPort int) {
	blockPeerMap[strings.ToLower(peerIP)] = BlockPeerInfoStruct { Timestamp: currentTimestamp, Port: peerPort }
}
func IsBlockedPeer(peerIP string, peerPort int, updateTimestamp bool) bool {
	if blockPeer, exist := blockPeerMap[peerIP]; exist {
		if !useNewBanPeersMethod || blockPeer.Port < 0 || blockPeer.Port == peerPort {
			if updateTimestamp {
				blockPeer.Timestamp = currentTimestamp
			}
			return true
		}
	}
	return false
}
func IsIPTooHighUploaded(ipInfo IPInfoStruct, lastIPInfo IPInfoStruct) float64 {
	var totalUploaded int64 = 0
	for torrentInfoHash, torrentUploaded := range ipInfo.TorrentUploaded {
		if lastTorrentUploaded, exist := lastIPInfo.TorrentUploaded[torrentInfoHash]; !exist {
			totalUploaded += torrentUploaded
		} else {
			totalUploaded += (torrentUploaded - lastTorrentUploaded)
		}
	}
	var totalUploadedMB float64 = (float64(totalUploaded) / 1024 / 1024)
	if totalUploadedMB > float64(config.IPUpCheckIncrementMB) {
		return totalUploadedMB
	}
	return 0
}
func IsProgressNotMatchUploaded(torrentTotalSize int64, clientProgress float64, clientUploaded int64) bool {
	if config.BanByProgressUploaded && torrentTotalSize > 0 && clientProgress >= 0 && clientUploaded > 0 {
		/*
		条件 1. 若客户端对 Peer 上传已大于等于 Torrnet 大小的 2%;
		条件 2. 但 Peer 报告进度乘以下载量再乘以一定防误判倍率, 却比客户端上传量还小;
		若满足以上条件, 则认为 Peer 是有问题的.
		e.g.:
		若 torrentTotalSize: 100GB, clientProgress: 1% (0.01), clientUploaded: 6GB, config.BanByPUStartPrecent: 2 (0.02), config.BanByPUAntiErrorRatio: 5;
		判断条件 1:
		torrentTotalSize * config.BanByPUStartPrecent = 100GB * 0.02 = 2GB, clientUploaded = 6GB >= 2GB
		满足此条件;
		判断条件 2:
		torrentTotalSize * clientProgress * config.BanByPUAntiErrorRatio = 100GB * 0.01 * 5 = 5GB, 5GB < clientUploaded = 6GB
		满足此条件;
		则该 Peer 将被封禁, 由于其报告进度为 1%, 算入 config.BanByPUAntiErrorRatio 滞后防误判倍率后为 5% (5GB), 但客户端实际却已上传 6GB.
		*/
		startUploaded := (float64(torrentTotalSize) * (float64(config.BanByPUStartPrecent) / 100))
		peerReportDownloaded := (float64(torrentTotalSize) * clientProgress);
		if (clientUploaded / 1024 / 1024) >= int64(config.BanByPUStartMB) && float64(clientUploaded) >= startUploaded && (peerReportDownloaded * float64(config.BanByPUAntiErrorRatio)) < float64(clientUploaded) {
			return true
		}
	}
	return false
}
func IsProgressNotMatchUploaded_Relative(peerInfo PeerInfoStruct, lastPeerInfo PeerInfoStruct) float64 {
	// 若客户端对 Peer 上传已大于 0, 且相对上传量大于起始上传量, 则继续判断.
	var relativeUploaded float64 = (float64(peerInfo.Uploaded - lastPeerInfo.Uploaded) / 1024 / 1024)
	if peerInfo.Uploaded > 0 && relativeUploaded > float64(config.BanByRelativePUStartMB) {
		relativeUploadedPrecent := (float64(lastPeerInfo.Uploaded) / float64(peerInfo.Uploaded))
		// 若相对上传百分比大于起始百分比, 则继续判断.
		if relativeUploadedPrecent > (float64(config.BanByRelativePUStartPrecent) / 100) {
			// 若相对上传百分比大于 Peer 报告进度乘以一定防误判倍率, 则认为 Peer 是有问题的.
			var peerReportProgress float64 = 0
			if peerInfo.Progress > 0 {
				peerReportProgress = (lastPeerInfo.Progress / peerInfo.Progress)
			}
			if relativeUploadedPrecent > (peerReportProgress * float64(config.BanByRelativePUAntiErrorRatio)) {
				return relativeUploaded;
			}
		}
	}
	return 0
}
func Fetch(url string) []byte {
	response, err := httpClient.Get(url)
	if err != nil {
		Log("Fetch", "请求时发生了错误: %s", true, err.Error())
		return nil
	}
	if response.StatusCode == 403 && !Login() {
		Log("Fetch", "请求时发生了错误: 认证失败", true)
		return nil
	}
	if response.StatusCode == 404 {
		Log("Fetch", "请求时发生了错误: 资源不存在", true)
		return nil
	}
	response, err = httpClient.Get(url)
	if err != nil {
		Log("Fetch", "请求时发生了错误: %s", true, err.Error())
		return nil
	}
	defer response.Body.Close()

	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		Log("Fetch", "读取时发生了错误: %s", true, err.Error())
		return nil
	}

	return responseBody
}
func Submit(url string, postdata string) []byte {
	response, err := httpClient.Post(url, "application/x-www-form-urlencoded", strings.NewReader(postdata))
	if err != nil {
		Log("Submit", "请求时发生了错误: %s", true, err.Error())
		return nil
	}
	if response.StatusCode == 403 && !Login() {
		Log("Submit", "请求时发生了错误: 认证失败", true)
		return nil
	}
	response, err = httpClient.Post(url, "application/x-www-form-urlencoded", strings.NewReader(postdata))
	if err != nil {
		Log("Submit", "请求时发生了错误: %s", true, err.Error())
		return nil
	}
	defer response.Body.Close()

	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		Log("Submit", "读取时发生了错误", true)
		return nil
	}

	return responseBody
}
func Login() bool {
	if config.QBUsername == "" {
		return true
	}
	loginParams := url.Values {}
	loginParams.Set("username", config.QBUsername)
	loginParams.Set("password", config.QBPassword)
	loginResponseBody := Submit(config.QBURL + "/api/v2/auth/login", loginParams.Encode())
	if loginResponseBody == nil {
		Log("Login", "登录时发生了错误", true)
		return false
	}

	loginResponseBodyStr := strings.TrimSpace(string(loginResponseBody))
	if loginResponseBodyStr == "Ok." {
		Log("Login", "登录成功", true)
		return true
	} else if loginResponseBodyStr == "Fails." {
		Log("Login", "登录失败: 账号或密码错误", true)
	} else {
		Log("Login", "登录失败: %s", true, loginResponseBodyStr)
	}
	return false
}
func FetchMaindata() *MainDataStruct {
	maindataResponseBody := Fetch(config.QBURL + "/api/v2/sync/maindata?rid=0")
	if maindataResponseBody == nil {
		Log("FetchMaindata", "发生错误", true)
		return nil
	}

	var mainDataResult MainDataStruct
	if err := json.Unmarshal(maindataResponseBody, &mainDataResult); err != nil {
		Log("FetchMaindata", "解析时发生了错误: %s", true, err.Error())
		return nil
	}

	Log("Debug-FetchMaindata", "完整更新: %s", false, strconv.FormatBool(mainDataResult.FullUpdate))

	return &mainDataResult
}
func FetchTorrentPeers(infoHash string) *TorrentPeersStruct {
	torrentPeersResponseBody := Fetch(config.QBURL + "/api/v2/sync/torrentPeers?rid=0&hash=" + infoHash)
	if torrentPeersResponseBody == nil {
		Log("FetchTorrentPeers", "发生错误", true)
		return nil
	}

	var torrentPeersResult TorrentPeersStruct
	if err := json.Unmarshal(torrentPeersResponseBody, &torrentPeersResult); err != nil {
		Log("FetchTorrentPeers", "解析时发生了错误: %s", true, err.Error())
		return nil
	}

	Log("Debug-FetchTorrentPeers", "完整更新: %s", false, strconv.FormatBool(torrentPeersResult.FullUpdate))

	return &torrentPeersResult
}
func GenBlockPeersStr() string {
	ip_ports := ""
	if useNewBanPeersMethod {
		for peerIP, peerInfo := range blockPeerMap {
			if peerInfo.Port == -1 {
				for port := 0; port <= 65535; port++ {
					ip_ports += peerIP + ":" + strconv.Itoa(port) + "|"
				}
			} else {
				ip_ports += peerIP + ":" + strconv.Itoa(peerInfo.Port) + "|"
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
		banResponseBody = Submit(config.QBURL + "/api/v2/transfer/banPeers", banIPPortsStr)
	} else {
		banIPPortsStr = url.QueryEscape("{\"banned_IPs\": \"" + banIPPortsStr + "\"}")
		banResponseBody = Submit(config.QBURL + "/api/v2/app/setPreferences", "json=" + banIPPortsStr)
	}
	if banResponseBody == nil {
		Log("SubmitBlockPeer", "发生错误", true)
	}
}
func ClearBlockPeer() int {
	cleanCount := 0
	if config.CleanInterval == 0 || (lastCleanTimestamp + int64(config.CleanInterval) < currentTimestamp) {
		for clientIP, clientInfo := range blockPeerMap {
			if currentTimestamp > (clientInfo.Timestamp + int64(config.BanTime)) {
				cleanCount++
				delete(blockPeerMap, clientIP)
			}
		}
		if cleanCount != 0 {
			lastCleanTimestamp = currentTimestamp
			Log("ClearBlockPeer", "已清理过期客户端: %d 个", true, cleanCount)
		}
	}
	return cleanCount
}
func CheckTorrent(torrentInfoHash string, torrentInfo TorrentStruct) (int, *TorrentPeersStruct) {
	Log("Debug-CheckTorrent", "%s", false, torrentInfoHash)
	if torrentInfoHash == "" {
		return -1, nil
	}
	if torrentInfo.NumLeechs < 1 {
		return -2, nil
	}
	torrentPeers := FetchTorrentPeers(torrentInfoHash)
	if torrentPeers == nil {
		return -3, nil
	}
	return 0, torrentPeers
}
func CheckPeer(peer PeerStruct, torrentInfoHash string, torrentTotalSize int64) int {
	if peer.IP == "" || peer.Client == "" || CheckPrivateIP(peer.IP) {
		return -1
	}
	if IsBlockedPeer(peer.IP, peer.Port, true) {
		Log("Debug-CheckPeer_IgnorePeer (Blocked)", "%s:%d %s", false, peer.IP, peer.Port, peer.Client)
		if peer.Port == -2 {
			return 4
		}
		if peer.Port == -1 {
			return 3
		}
		return 2
	}
	Log("Debug-CheckPeer", "%s %s", false, peer.IP, peer.Client)
	if IsProgressNotMatchUploaded(torrentTotalSize, peer.Progress, peer.Uploaded) {
		Log("CheckPeer_AddBlockPeer (Bad-Progress_Uploaded)", "%s:%d %s (TorrentTotalSize: %.2f MB, Progress: %.2f%%, Uploaded: %.2f MB)", true, peer.IP, peer.Port, peer.Client, (float64(torrentTotalSize) / 1024 / 1024), (peer.Progress * 100), (float64(peer.Uploaded) / 1024 / 1024))
		AddBlockPeer(peer.IP, peer.Port)
		return 1
	}
	for _, v := range blockListCompiled {
		if v.MatchString(peer.Client) {
			Log("CheckPeer_AddBlockPeer (Bad-Client)", "%s:%d %s", true, peer.IP, peer.Port, peer.Client)
			AddBlockPeer(peer.IP, peer.Port)
			return 1
		}
	}
	AddIPInfo(peer.IP, torrentInfoHash, peer.Uploaded)
	AddPeerInfo(peer.IP, peer.Port, peer.Progress, peer.Uploaded)
	return 0
}
func CheckAllIP(lastIPMap map[string]IPInfoStruct) int {
	if config.IPUploadedCheck && len(lastIPMap) > 0 && currentTimestamp > (lastIPCleanTimestamp + int64(config.IPUpCheckInterval)) {
		blockCount := 0
		for ip, ipInfo := range ipMap {
			if IsBlockedPeer(ip, -1, false) {
				continue
			}
			if lastIPInfo, exist := lastIPMap[ip]; exist {
				if uploadDuring := IsIPTooHighUploaded(ipInfo, lastIPInfo); uploadDuring > 0 {
					Log("CheckAllIP_AddBlockPeer (Too high uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, ip, -1, uploadDuring)
					blockCount++
					AddBlockPeer(ip, -1)
				}
			}
		}
		lastIPCleanTimestamp = currentTimestamp
		ipMap = make(map[string]IPInfoStruct)
		return blockCount
	}
	return 0
}
func CheckAllPeer(lastPeerMap map[string]PeerInfoStruct) int {
	if (config.MaxIPPortCount > 0 || config.BanByRelativeProgressUploaded) && len(lastPeerMap) > 0 && currentTimestamp > (lastPeerCleanTimestamp + int64(config.PeerMapCleanInterval)) {
		blockCount := 0
		peerMapLoop:
		for ip, peerInfo := range peerMap {
			if IsBlockedPeer(ip, -1, false) || IsBlockedPeer(ip, -2, false) {
				continue
			}
			for port := range peerInfo.Port {
				if IsBlockedPeer(ip, port, false) {
					continue peerMapLoop
				}
			}
			if config.MaxIPPortCount > 0 {
				if len(peerInfo.Port) > int(config.MaxIPPortCount) {
					Log("CheckAllPeer_AddBlockPeer (Too many ports)", "%s:%d", true, ip, -1)
					AddBlockPeer(ip, -1)
					continue
				}
			}
			if config.BanByRelativeProgressUploaded {
				if lastPeerInfo, exist := lastPeerMap[ip]; exist {
					if uploadDuring := IsProgressNotMatchUploaded_Relative(peerInfo, lastPeerInfo); uploadDuring > 0 {
						Log("CheckAllPeer_AddBlockPeer (Bad-Relative_Progress_Uploaded)", "%s:%d (UploadDuring: %.2f MB)", true, ip, -2, uploadDuring)
						blockCount++
						AddBlockPeer(ip, -2)
					}
				}
			}
		}
		lastPeerCleanTimestamp = currentTimestamp
		peerMap = make(map[string]PeerInfoStruct)
		return blockCount
	}
	return 0
}
func Task() {
	metadata := FetchMaindata()
	if metadata == nil {
		return
	}

	cleanCount := ClearBlockPeer()
	blockCount := 0
	ipBlockCount := 0
	emptyHashCount := 0
	noLeechersCount := 0
	badTorrentInfoCount := 0
	badPeersCount := 0
	lastIPMap := ipMap
	lastPeerMap := peerMap

	for torrentInfoHash, torrentInfo := range metadata.Torrents {
		torrentStatus, torrentPeers := CheckTorrent(torrentInfoHash, torrentInfo)
		switch torrentStatus {
			case -1:
				emptyHashCount++
			case -2:
				noLeechersCount++
			case -3:
				badTorrentInfoCount++
			case 0:
				for _, peers := range torrentPeers.Peers {
					peerStatus := CheckPeer(peers, torrentInfoHash, torrentInfo.TotalSize)
					switch peerStatus {
						case 3:
							ipBlockCount++
						case 1:
							blockCount++
						case -1:
							badPeersCount++
					}
				}
		}
		if config.SleepTime != 0 {
			time.Sleep(time.Duration(config.SleepTime) * time.Millisecond)
		}
	}

	currentIPBlockCount := CheckAllIP(lastIPMap)
	ipBlockCount += currentIPBlockCount
	blockCount += CheckAllPeer(lastPeerMap)

	Log("Debug-Task_IgnoreEmptyHashCount", "%d", false, emptyHashCount)
	Log("Debug-Task_IgnoreNoLeechersCount", "%d", false, noLeechersCount)
	Log("Debug-Task_IgnoreBadTorrentInfoCount", "%d", false, badTorrentInfoCount)
	Log("Debug-Task_IgnoreBadPeersCount", "%d", false, badPeersCount)
	if cleanCount != 0 || blockCount != 0 {
		peersStr := GenBlockPeersStr()
		Log("Debug-Task_GenBlockPeersStr", "%s", false, peersStr)
		SubmitBlockPeer(peersStr)
		Log("Task", "此次封禁客户端: %d 个, 当前封禁客户端: %d 个, 此次封禁 IP 地址: %d 个, 当前封禁 IP 地址: %d 个", true, blockCount, len(blockPeerMap), currentIPBlockCount, ipBlockCount)
	}
}
func RunConsole() {
	flag.StringVar(&configFilename, "c", "config.json", "配置文件路径")
	flag.Parse()
	if !LoadConfig() {
		Log("RunConsole", "读取配置文件失败或不完整", false)
		InitConfig()
	}
	if !Login() {
		Log("RunConsole", "认证失败", true)
		return
	}
	SubmitBlockPeer("")
	Log("RunConsole", "程序已启动", true)
	for range time.Tick(time.Duration(config.Interval) * time.Second) {
		currentTimestamp = time.Now().Unix()
		if LoadConfig() {
			Task()
		}
	}
}
