package main

import (
	"os"
	"fmt"
	"time"
	"strings"
	"strconv"
	"reflect"
	"net/http"
	"io/ioutil"
	"encoding/json"
)

type BlockClientInfoStruct struct {
	Timestamp int64
	PeerID string
}
type TorrentStruct struct {
}
type MainDataStruct struct {
	Rid int64 `json:"rid"`
	FullUpdate bool `json:"full_update"`
	Torrents map[string]TorrentStruct `json:"torrents"`
}
type ConfigStruct struct {
	Debug bool
	QBURL string
}

var blockClientMap = make(map[string]BlockClientInfoStruct)
var lastMaindataRid int64 = 0
var httpClient = http.Client {
    Timeout: 8 * time.Second,
}
var config = ConfigStruct{ Debug: false, QBURL: "http://127.0.0.1:990" }
var configFilename = "config.json"
var configLastMod int64 = 0;

func Log(module string, str string, args ...interface{}) {
	if !config.Debug && strings.HasPrefix(module, "Debug") {
		return
	}
	logStr := fmt.Sprintf("[" + module + "] " + str + ".\n", args...)
	fmt.Print(logStr)
}
func LoadConfig() bool {
	configFileStat, err := os.Stat(configFilename)
	if err != nil {
		Log("Debug-LoadConfig", "读取配置文件元数据时发生了错误: " + err.Error())
	}
	tmpConfigLastMod := configFileStat.ModTime().Unix()
	if tmpConfigLastMod <= configLastMod {
		return true
	}
	if configLastMod != 0 {
		Log("Debug-LoadConfig", "发现配置文件更改, 正在进行热重载")
	}
	configLastMod = tmpConfigLastMod
	configFile, err := ioutil.ReadFile(configFilename)
	if err != nil {
		Log("LoadConfig", "读取配置文件时发生了错误: " + err.Error())
		return false
	}
	json.Unmarshal(configFile, &config)
	Log("LoadConfig", "读取配置文件成功")
	t := reflect.TypeOf(config)
	v := reflect.ValueOf(config)
	for k := 0; k < t.NumField(); k++ {
		Log("LoadConfig-Current", "%v: %v", t.Field(k).Name, v.Field(k).Interface())
	}
	return true
}
func AddBlockClient(clientIP string, clientPeerID string) {
	blockClientMap[strings.ToLower(clientIP)] = BlockClientInfoStruct{ Timestamp: time.Now().Unix(), PeerID: clientPeerID }
}
func FetchMaindata(rid int64) *MainDataStruct {
	maindataResponse, err := httpClient.Get(config.QBURL + "/api/v2/sync/maindata?rid=" + strconv.FormatInt(rid, 10))
	if err != nil {
		Log("Maindata", "请求时发生了错误: " + err.Error())
		return nil
	}
	defer maindataResponse.Body.Close()
	maindataResponseBody, err := ioutil.ReadAll(maindataResponse.Body)

	var mainDataResult MainDataStruct
	if err := json.Unmarshal(maindataResponseBody, &mainDataResult); err != nil {
		Log("Maindata", "解析时发生了错误")
		return nil
	}
	lastMaindataRid = mainDataResult.Rid
	Log("Debug-Maindata", fmt.Sprintf("rid: %d, 完整更新: %s", lastMaindataRid, strconv.FormatBool(mainDataResult.FullUpdate)))

	return &mainDataResult
}
func FetchTorrentPeers(rid int64, infoHash string) string {
	return ""
}
func Task() {
	cleanCount := 0
	for clientIP, clientInfo := range blockClientMap {
		if clientInfo.Timestamp + 86400 < time.Now().Unix() {
			cleanCount++
			delete(blockClientMap, clientIP)
		}
	}
	if cleanCount != 0 {
		Log("Task", "已清理过期客户端: %d 个", cleanCount)
	}

	metadata := FetchMaindata(lastMaindataRid)
	if metadata == nil {
		return
	}
	//var lastTorrentRid int64 = 0
	for infoHash, _ := range metadata.Torrents {
		Log("Debug-Task", "%s", infoHash)
		if infoHash != "" {
			//torrentPeers := FetchTorrentPeers(lastTorrentRid, infoHash)
		}
	}
}
func main() {
	Log("Main", "程序已启动")
	for range time.Tick(2 * time.Second) {
		LoadConfig()
		Task()
	}
}
