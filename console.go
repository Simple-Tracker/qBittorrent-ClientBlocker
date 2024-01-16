package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type BlockPeerInfoStruct struct {
	Timestamp int64
	Name      string
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
	Client   string
	Progress float64
	Uploaded int64
}
type TorrentPeersStruct struct {
	FullUpdate bool                  `json:"full_update"`
	Peers      map[string]PeerStruct `json:"peers"`
}
type ConfigStruct struct {
	Debug                 bool
	Interval              uint32
	CleanInterval         uint32
	BanTime               uint32
	SleepTime             uint32
	Timeout               uint32
	BanByProgressUploaded bool
	BanByPUStartMB        uint32
	BanByPUStartPrecent   uint32
	BanByPUAntiErrorRatio uint32
	LongConnection        bool
	LogToFile             bool
	LogDebug              bool
	QBURL                 string
	QBUsername            string
	QBPassword            string
	BlockList             []string
}

var todayStr = ""
var currentTimestamp int64 = 0
var lastCleanTimestamp int64 = 0
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
	Debug:                 false,
	Interval:              2,
	CleanInterval:         3600,
	BanTime:               86400,
	SleepTime:             20,
	Timeout:               6,
	BanByProgressUploaded: false,
	BanByPUStartMB:        10,
	BanByPUStartPrecent:   2,
	BanByPUAntiErrorRatio: 5,
	LongConnection:        true,
	LogToFile:             true,
	LogDebug:              false,
	QBURL:                 "http://127.0.0.1:990",
	QBUsername:            "",
	QBPassword:            "",
	BlockList:             []string {},
}
var configFilename = "config.json"
var configLastMod int64 = 0
var logFile *os.File

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
func GetDateTime(withTime bool) string {
	formatStr := "2006-01-02"
	if withTime {
		formatStr += " 15:04:05"
	}
	return time.Now().Format(formatStr)
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
	if config.BanTime < config.CleanInterval {
		config.BanTime = config.CleanInterval
	}
	if config.BanTime < 1 {
		config.BanTime = 1
	}
	Log("LoadConfig", "读取配置文件成功", true)
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
	return true
}
func CheckPrivateIP(ip string) bool {
	ipParsed := net.ParseIP(ip)
	return ipParsed.IsPrivate()
}
func AddBlockPeer(clientIP string, clientName string) {
	blockPeerMap[strings.ToLower(clientIP)] = BlockPeerInfoStruct { Timestamp: currentTimestamp, Name: clientName }
}
func IsBlockedPeer(clientIP string, updateTimestamp bool) bool {
	if blockPeer, exist := blockPeerMap[clientIP]; exist {
		if updateTimestamp {
			blockPeer.Timestamp = currentTimestamp
		}
		return true
	}
	return false
}
func IsProgressNotMatchUploaded(torrentTotalSize int64, clientProgress float64, clientUploaded int64) bool {
	if config.BanByProgressUploaded && torrentTotalSize > 0 && clientProgress > 0 && clientUploaded > 0 {
		/*
		条件 1. 若客户端上传已大于等于 Torrnet 大小的 2%;
		条件 2. 但 Peer 实际进度乘以下载量再乘以一定防误判倍率, 却比客户端上传量还小;
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
		startUploaded := (float64(torrentTotalSize) * float64(config.BanByPUStartPrecent / 100))
		peerReportDownloaded := (float64(torrentTotalSize) * clientProgress);
		if (clientUploaded / 1024 / 1024) >= int64(config.BanByPUStartMB) && float64(clientUploaded) >= startUploaded && (peerReportDownloaded * float64(config.BanByPUAntiErrorRatio)) < float64(clientUploaded) {
			return true
		}
	}
	return false
}
func GenBlockPeersStr() string {
	ips := ""
	for k := range blockPeerMap {
		ips += k + "\n"
	}
	return ips
}
func Login() bool {
	if config.QBUsername == "" {
		return true
	}
	loginParams := url.Values {}
	loginParams.Set("QBUsername", config.QBUsername)
	loginParams.Set("QBPassword", config.QBPassword)
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
		Log("Login", "登录失败: " + loginResponseBodyStr, true)
	}
	return false
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
func SubmitBlockPeers(banIPsStr string) {
	banIPsStr = url.QueryEscape("{\"banned_IPs\": \"" + banIPsStr + "\"}")
	banResponseBody := Submit(config.QBURL + "/api/v2/app/setPreferences", "json=" + banIPsStr)
	if banResponseBody == nil {
		Log("SubmitBlockPeers", "发生错误", true)
	}
}
func Task() {
	cleanCount := 0
	if config.CleanInterval == 0 || (lastCleanTimestamp + int64(config.CleanInterval) < currentTimestamp) {
		for clientIP, clientInfo := range blockPeerMap {
			if clientInfo.Timestamp + int64(config.BanTime) < currentTimestamp {
				cleanCount++
				delete(blockPeerMap, clientIP)
			}
		}
		if cleanCount != 0 {
			lastCleanTimestamp = currentTimestamp
			Log("Task", "已清理过期客户端: %d 个", true, cleanCount)
		}
	}

	metadata := FetchMaindata()
	if metadata == nil {
		return
	}

	blockCount := 0
	emptyHashCount := 0
	noLeechersCount := 0
	badPeerInfoCount := 0
	for torrentInfoHash, torrentInfoArr := range metadata.Torrents {
		if torrentInfoArr.NumLeechs < 1 {
			noLeechersCount++
			continue;
		}
		Log("Debug-Task_CheckHash", "%s", false, torrentInfoHash)
		if torrentInfoHash == "" {
			emptyHashCount++
			continue
		}
		torrentPeers := FetchTorrentPeers(torrentInfoHash)
		if torrentPeers == nil {
			badPeerInfoCount++
			continue
		}
		for _, peerInfo := range torrentPeers.Peers {
			if peerInfo.IP == "" || peerInfo.Client == "" || CheckPrivateIP(peerInfo.IP) {
				badPeerInfoCount++
				continue
			}
			if IsBlockedPeer(peerInfo.IP, true) {
				Log("Debug-Task_IgnorePeer (Blocked)", "%s %s", false, peerInfo.IP, peerInfo.Client)
				continue
			}
			Log("Debug-Task_CheckPeer", "%s %s", false, peerInfo.IP, peerInfo.Client)
			if IsProgressNotMatchUploaded(torrentInfoArr.TotalSize, peerInfo.Progress, peerInfo.Uploaded) {
				blockCount++
				Log("Task_AddBlockPeer (Bad-Progess_Uploaded)", "%s %s (TorrentTotalSize: %d, Progress: %.2f%%, Uploaded: %d)", true, peerInfo.IP, peerInfo.Client, torrentInfoArr.TotalSize, (peerInfo.Progress * 100), peerInfo.Uploaded)
				AddBlockPeer(peerInfo.IP, peerInfo.Client)
				continue
			}
			for _, v := range blockListCompiled {
				if v.MatchString(peerInfo.Client) {
					blockCount++
					Log("Task_AddBlockPeer (Bad-Client)", "%s %s", true, peerInfo.IP, peerInfo.Client)
					AddBlockPeer(peerInfo.IP, peerInfo.Client)
					break
				}
			}
		}
		if config.SleepTime != 0 {
			time.Sleep(time.Duration(config.SleepTime) * time.Millisecond)
		}
	}
	Log("Debug-Task_IgnoreEmptyHashCount", "%d", false, emptyHashCount)
	Log("Debug-Task_IgnoreNoLeechersCount", "%d", false, noLeechersCount)
	Log("Debug-Task_IgnoreBadPeerInfoCount", "%d", false, badPeerInfoCount)
	if cleanCount != 0 || blockCount != 0 {
		peersStr := GenBlockPeersStr()
		Log("Debug-Task_GenBlockPeersStr", "%s", false, peersStr)
		SubmitBlockPeers(peersStr)
		Log("Task", "此次封禁客户端: %d 个, 当前封禁客户端: %d 个", true, blockCount, len(blockPeerMap))
	}
}
func RunConsole() {
	if !LoadConfig() {
		Log("Main", "读取配置文件失败或不完整", false)
	}
	if !Login() {
		Log("Main", "认证失败", true)
		return
	}
	SubmitBlockPeers("")
	Log("Main", "程序已启动", true)
	for range time.Tick(time.Duration(config.Interval) * time.Second) {
		currentTimestamp = time.Now().Unix()
		if LoadConfig() {
			Task()
		}
	}
}
