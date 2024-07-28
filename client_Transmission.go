package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Tr_RequestStruct struct {
	Method string      `json:"method"`
	Args   interface{} `json:"arguments"`
}
type Tr_ResponseStruct struct {
	Result string `json:"result"`
}
type Tr_TorrentsResponseStruct struct {
	Result string            `json:"result"`
	Args   Tr_TorrentsStruct `json:"arguments"`
}
type Tr_ArgsStruct struct {
	Field []string `json:"fields"`
}
type Tr_ArgTorrentsStruct struct {
	IDs   []string `json:"ids"`
	Field []string `json:"fields"`
}
type Tr_SessionSetStruct struct {
	BlocklistEnabled bool   `json:"blocklist-enabled"`
	BlocklistSize    int    `json:"blocklist-size"`
	BlocklistURL     string `json:"blocklist-url"`
}
type Tr_TorrentsStruct struct {
	Torrents []Tr_TorrentStruct `json:"torrents"`
}
type Tr_TorrentStruct struct {
	InfoHash  string          `json:"hashString"`
	TotalSize int64           `json:"totalSize"`
	Private   bool            `json:"private"`
	Peers     []Tr_PeerStruct `json:"peers"`
}
type Tr_PeerStruct struct {
	IP          string  `json:"address"`
	Port        int     `json:"port"`
	Client      string  `json:"clientName"`
	Progress    float64 `json:"progress"`
	IsUploading bool    `json:"isUploadingTo"`
	DlSpeed     int64   `json:"rateToClient"`
	UpSpeed     int64   `json:"rateToPeer"`
}

var Tr_csrfToken = ""
var Tr_ipfilterStr = ""
var Tr_jsonHeader = map[string]string{"Content-Type": "application/json"}

func Tr_InitClient() {
	go StartServer()
}
func Tr_ProcessHTTP(w http.ResponseWriter, r *http.Request) bool {
	if strings.SplitN(r.RequestURI, "?", 2)[0] == "/ipfilter.dat" {
		w.WriteHeader(200)
		w.Write([]byte(Tr_ipfilterStr))

		return true
	}

	return false
}
func Tr_SetURL() bool {
	return false
}
func Tr_DetectVersion() bool {
	detectJSON, err := json.Marshal(Tr_RequestStruct{Method: "session-get", Args: Tr_ArgsStruct{Field: []string{"version"}}})
	if err != nil {
		Log("DetectVersion", GetLangText("Error-GenJSON"), true, err.Error())
		return false
	}

	detectStatusCode, _, _ := Submit(config.ClientURL, string(detectJSON), false, false, &Tr_jsonHeader)
	return (detectStatusCode == 200 || detectStatusCode == 409)
}
func Tr_Login() bool {
	// Transmission 通过 Basic Auth 进行认证, 因此实际处理 CSRF 请求以避免 409 响应.
	loginJSON, err := json.Marshal(Tr_RequestStruct{Method: "session-get"})
	if err != nil {
		Log("Login", GetLangText("Error-GenJSON"), true, err.Error())
		return false
	}

	Submit(config.ClientURL, string(loginJSON), false, true, nil)

	if Tr_csrfToken == "" {
		Log("Login", GetLangText("Error-Login"), true)
		return false
	}

	return true
}
func Tr_SetCSRFToken(csrfToken string) {
	Tr_csrfToken = csrfToken
	Log("SetCSRFToken", GetLangText("Success-SetCSRFToken"), true, csrfToken)
}
func Tr_FetchTorrents() *Tr_TorrentsStruct {
	loginJSON, err := json.Marshal(Tr_RequestStruct{Method: "torrent-get", Args: Tr_ArgsStruct{Field: []string{"hashString", "totalSize", "isPrivate", "peers"}}})
	if err != nil {
		Log("FetchTorrents", GetLangText("Error-GenJSON"), true, err.Error())
		return nil
	}

	_, _, torrentsResponseBody := Submit(config.ClientURL, string(loginJSON), true, true, &Tr_jsonHeader)
	if torrentsResponseBody == nil {
		Log("FetchTorrents", GetLangText("Error"), true)
		return nil
	}

	var torrentsResponse Tr_TorrentsResponseStruct
	if err := json.Unmarshal(torrentsResponseBody, &torrentsResponse); err != nil {
		Log("FetchTorrents", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	if torrentsResponse.Result != "success" {
		Log("FetchTorrents", GetLangText("Error-Parse"), true, torrentsResponse.Result)
		return nil
	}

	return &torrentsResponse.Args
}

func Tr_RestartTorrentByMap(blockPeerMap map[string]BlockPeerInfoStruct) {
	peerInfoHashes := []string{}
	for _, peerInfo := range blockPeerMap {
		peerInfoHashes = append(peerInfoHashes, peerInfo.InfoHash)
	}

	if len(peerInfoHashes) <= 0 {
		return
	}

	stopJSON, err := json.Marshal(Tr_RequestStruct{Method: "torrent-stop", Args: Tr_ArgTorrentsStruct{IDs: peerInfoHashes}})
	if err != nil {
		Log("RestartTorrentByMap", GetLangText("Error-GenJSON"), true, err.Error())
		return
	}

	stopStatusCode, _, _ := Submit(config.ClientURL, string(stopJSON), true, true, &Tr_jsonHeader)
	if stopStatusCode != 200 {
		Log("RestartTorrentByMap", GetLangText("Error-RestartTorrentByMap_Stop"), true, err.Error())
		return
	}

	Log("RestartTorrentByMap", GetLangText("Debug-RestartTorrentByMap_Wait"), true, config.Interval)
	time.Sleep(time.Duration(config.RestartInterval) * time.Second)

	startJSON, err := json.Marshal(Tr_RequestStruct{Method: "torrent-start", Args: Tr_ArgTorrentsStruct{IDs: peerInfoHashes}})
	if err != nil {
		Log("RestartTorrentByMap", GetLangText("Error-GenJSON"), true, err.Error())
		return
	}

	startStatusCode, _, _ := Submit(config.ClientURL, string(startJSON), true, true, &Tr_jsonHeader)
	if startStatusCode != 200 {
		Log("RestartTorrentByMap", GetLangText("Error-RestartTorrentByMap_Start"), true, err.Error())
		return
	}
}
func Tr_SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	ipfilterCount, ipfilterStr := GenIPFilter(2, blockPeerMap)
	Tr_ipfilterStr = ipfilterStr

	blocklistURL := ""
	if strings.Contains(config.Listen, ".") {
		blocklistURL = "http://" + config.Listen
	} else {
		blocklistURL = "http://127.0.0.1" + config.Listen
	}
	blocklistURL += "/ipfilter.dat?t=" + strconv.FormatInt(currentTimestamp, 10)

	sessionSetJSON, err := json.Marshal(Tr_RequestStruct{Method: "session-set", Args: Tr_SessionSetStruct{BlocklistEnabled: true, BlocklistSize: ipfilterCount, BlocklistURL: blocklistURL}})
	if err != nil {
		Log("SubmitBlockPeer", GetLangText("Error-GenJSON"), true, err.Error())
		return false
	}

	_, _, sessionResponseBody := Submit(config.ClientURL, string(sessionSetJSON), true, true, &Tr_jsonHeader)
	if sessionResponseBody == nil {
		Log("SubmitBlockPeer", GetLangText("Error"), true)
		return false
	}

	var sessionResponse Tr_ResponseStruct
	if err := json.Unmarshal(sessionResponseBody, &sessionResponse); err != nil {
		Log("SubmitBlockPeer", GetLangText("Error-Parse"), true, err.Error())
		return false
	}

	if sessionResponse.Result != "success" {
		Log("SubmitBlockPeer", GetLangText("Error-Parse"), true, sessionResponse.Result)
		return false
	}

	blocklistUpdateJSON, err := json.Marshal(Tr_RequestStruct{Method: "blocklist-update"})
	if err != nil {
		Log("SubmitBlockPeer", GetLangText("Error-GenJSON"), true, err.Error())
		return false
	}

	_, _, blocklistUpdateResponseBody := Submit(config.ClientURL, string(blocklistUpdateJSON), true, true, &Tr_jsonHeader)
	if blocklistUpdateResponseBody == nil {
		Log("SubmitBlockPeer", GetLangText("Error"), true)
		return false
	}

	var blocklistUpdateResponse Tr_ResponseStruct
	if err := json.Unmarshal(blocklistUpdateResponseBody, &blocklistUpdateResponse); err != nil {
		Log("SubmitBlockPeer", GetLangText("Error-Parse"), true, err.Error())
		return false
	}

	if blocklistUpdateResponse.Result != "success" {
		Log("SubmitBlockPeer", GetLangText("Error-Parse"), true, blocklistUpdateResponse.Result)
		return false
	}

	Tr_RestartTorrentByMap(blockPeerMap)

	return true
}
