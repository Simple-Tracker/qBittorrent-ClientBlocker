package main

import (
	"fmt"
	"time"
	"strings"
	"strconv"
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

var blockClientMap = make(map[string]BlockClientInfoStruct)
var lastMaindataRid int64 = 0
var qBURL string = "http://127.0.0.1:990"
var httpClient = http.Client {
    Timeout: 8 * time.Second,
}
var debug bool = true

func Log(module string, str string, args ...interface{}) {
	if !debug && strings.HasPrefix(module, "Debug") {
		return
	}
	logStr := fmt.Sprintf("[" + module + "] " + str + ".\n", args...)
	fmt.Print(logStr)
}
func AddBlockClient(clientIP string, clientPeerID string) {
	blockClientMap[strings.ToLower(clientIP)] = BlockClientInfoStruct{ Timestamp: time.Now().Unix(), PeerID: clientPeerID }
}
func FetchMaindata(rid int64) *MainDataStruct {
	maindataResponse, err := httpClient.Get(qBURL + "/api/v2/sync/maindata?rid=" + strconv.FormatInt(rid, 10))
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
func FetchTorrentPeers(rid int64, infoHash string) {

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
	var lastTorrentRid int64 = 0
	for infoHash, _ := range metadata.Torrents {
		Log("Debug", "%s", infoHash)
		if infoHash != nil {
			torrentPeers := FetchTorrentPeers(lastTorrentRid, infoHash)
		}
	}
}
func main() {
	Log("Main", "程序已启动")
	for range time.Tick(2 * time.Second) {
		Task()
	}
}
