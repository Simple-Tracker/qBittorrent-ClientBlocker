package main

import (
	"os"
	"fmt"
	"net"
	"time"
	"regexp"
	"strings"
	"strconv"
	"reflect"
	"net/url"
	"net/http"
	"io/ioutil"
	"encoding/json"
)

type BlockPeerInfoStruct struct {
	Timestamp int64
	Name string
}
type MainDataStruct struct {
	FullUpdate bool `json:"full_update"`
	Torrents map[string]TorrentStruct `json:"torrents"`
}
type TorrentStruct struct {
}
type PeerStruct struct {
	IP string
	Client string
}
type TorrentPeersStruct struct {
	FullUpdate bool `json:"full_update"`
	Peers map[string]PeerStruct `json:"peers"`
}
type ConfigStruct struct {
	Debug bool
	Timeout int
	LogToFile bool
	QBURL string
	BlockList []string
}

var todayStr = ""
var currentTimestamp int64 = 0
var blockPeerMap = make(map[string]BlockPeerInfoStruct)
var blockListCompiled []*regexp.Regexp
var httpClient = http.Client {
    Timeout: 30 * time.Second,
}
var config = ConfigStruct{ Debug: false, Timeout: 30, LogToFile: true, QBURL: "http://127.0.0.1:990", BlockList: []string {} }
var configFilename = "config.json"
var configLastMod int64 = 0;
var logFile *os.File

func Log(module string, str string, logToFile bool, args ...interface{}) {
	if !config.Debug && strings.HasPrefix(module, "Debug") {
		return
	}
	logStr := fmt.Sprintf("[" + GetDateTime(true) + "][" + module + "] " + str + ".\n", args...)
	if logToFile && config.LogToFile && logFile != nil {
		if _, err := logFile.Write([]byte(logStr)); err != nil {
			Log("Log", "无法写入日志", false)
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
func ReloadLog() {
	tmpTodayStr := GetDateTime(false)
	if todayStr != tmpTodayStr {
		todayStr = tmpTodayStr
		logFile.Close()

		tLogFile, err := os.OpenFile("logs/" + todayStr + ".txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			tLogFile.Close()
			tLogFile = nil
			Log("ReloadLog", "无法访问日志", false)
		}
		logFile = tLogFile
	}
}
func LoadConfig() bool {
	configFileStat, err := os.Stat(configFilename)
	if err != nil {
		Log("Debug-LoadConfig", "读取配置文件元数据时发生了错误: " + err.Error(), false)
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
		Log("LoadConfig", "读取配置文件时发生了错误: " + err.Error(), false)
		return false
	}
	configLastMod = tmpConfigLastMod
	json.Unmarshal(configFile, &config)
	if config.LogToFile {
		os.Mkdir("logs", os.ModePerm)
		ReloadLog()
	}
	Log("LoadConfig", "读取配置文件成功", true)
	if config.Timeout != 30 {
		httpClient = http.Client {
		    Timeout: time.Duration(config.Timeout) * time.Second,
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
	blockPeerMap[strings.ToLower(clientIP)] = BlockPeerInfoStruct{ Timestamp: currentTimestamp, Name: clientName }
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
func GenBlockPeersStr() string {
	ips := ""
	for k := range blockPeerMap {
		ips += k + "\n"
	}
	return ips
}
func Fetch(url string) []byte {
	response, err := httpClient.Get(url)
	if err != nil {
		Log("Fetch", "请求时发生了错误: " + err.Error(), false)
		return nil
	}
	defer response.Body.Close()

	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		Log("Fetch", "读取时发生了错误", false)
		return nil
	}

	return responseBody
}
func Submit(url string, postdata string) []byte {
	response, err := httpClient.Post(url, "application/x-www-form-urlencoded", strings.NewReader(postdata))
	if err != nil {
		Log("Submit", "请求时发生了错误: " + err.Error(), false)
		return nil
	}
	defer response.Body.Close()

	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		Log("Submit", "读取时发生了错误", false)
		return nil
	}

	return responseBody
}
func FetchMaindata() *MainDataStruct {
	maindataResponseBody := Fetch(config.QBURL + "/api/v2/sync/maindata?rid=0")
	if maindataResponseBody == nil {
		Log("FetchMaindata", "发生错误", false)
		return nil
	}
	
	var mainDataResult MainDataStruct
	if err := json.Unmarshal(maindataResponseBody, &mainDataResult); err != nil {
		Log("FetchMaindata", "解析时发生了错误", false)
		return nil
	}

	Log("Debug-FetchMaindata", "完整更新: %s", false, strconv.FormatBool(mainDataResult.FullUpdate))

	return &mainDataResult
}
func FetchTorrentPeers(infoHash string) *TorrentPeersStruct {
	torrentPeersResponseBody := Fetch(config.QBURL + "/api/v2/sync/torrentPeers?rid=0&hash=" + infoHash)
	if torrentPeersResponseBody == nil {
		Log("FetchTorrentPeers", "发生错误", false)
		return nil
	}

	var torrentPeersResult TorrentPeersStruct
	if err := json.Unmarshal(torrentPeersResponseBody, &torrentPeersResult); err != nil {
		Log("FetchTorrentPeers", "解析时发生了错误", false)
		return nil
	}

	Log("Debug-FetchTorrentPeers", "完整更新: %s", false, strconv.FormatBool(torrentPeersResult.FullUpdate))

	return &torrentPeersResult
}
func SubmitBlockPeers(banIPsStr string) {
	banIPsStr = url.QueryEscape("{\"banned_IPs\": \"" + banIPsStr + "\"}")
	banResponseBody := Submit(config.QBURL + "/api/v2/app/setPreferences", "json=" + banIPsStr)
	if banResponseBody == nil {
		Log("SubmitBlockPeers", "发生错误", false)
	}
}
func Task() {
	cleanCount := 0
	for clientIP, clientInfo := range blockPeerMap {
		if clientInfo.Timestamp + 86400 < currentTimestamp {
			cleanCount++
			delete(blockPeerMap, clientIP)
		}
	}
	if cleanCount != 0 {
		Log("Task", "已清理过期客户端: %d 个", true, cleanCount)
	}

	metadata := FetchMaindata()
	if metadata == nil {
		return
	}

	blockCount := 0
	for infoHash, _ := range metadata.Torrents {
		Log("Debug-Task_CheckHash", "%s", false, infoHash)
		if infoHash == "" {
			continue
		}
		torrentPeers := FetchTorrentPeers(infoHash)
		for _, peerInfo := range torrentPeers.Peers {
			if peerInfo.IP == "" || peerInfo.Client == "" || CheckPrivateIP(peerInfo.IP) {
				continue
			}
			if IsBlockedPeer(peerInfo.IP, true) {
				Log("Debug-Task_IgnorePeer", "%s %s", false, peerInfo.IP, peerInfo.Client)
				continue
			}
			Log("Debug-Task_CheckPeer", "%s %s", false, peerInfo.IP, peerInfo.Client)
			for _, v := range blockListCompiled {
				if v.MatchString(peerInfo.Client) {
					blockCount++
					Log("Task_AddBlockPeer", "%s %s", true, peerInfo.IP, peerInfo.Client)
					AddBlockPeer(peerInfo.IP, peerInfo.Client)
					break
				}
			}
		}
	}
	if cleanCount != 0 || blockCount != 0 {
		peersStr := GenBlockPeersStr()
		Log("Debug-Task_GenBlockPeersStr", "%s", false, peersStr)
		SubmitBlockPeers(peersStr)
		Log("Task", "此次封禁客户端: %d 个, 当前封禁客户端: %d 个", true, blockCount, len(blockPeerMap))
	}
}
func main() {
	LoadConfig()
	SubmitBlockPeers("")
	Log("Main", "程序已启动", true)
	for range time.Tick(2 * time.Second) {
		currentTimestamp = time.Now().Unix()
		LoadConfig()
		Task()
	}
}
