package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// TRClient 实现了 Transmission 的客户端接口.
type TRClient struct{}

func (c *TRClient) GetClientType() string {
	return "Transmission"
}

func (c *TRClient) ConfigPath() string {
	return ""
}

func (c *TRClient) SetURL() bool {
	return Tr_SetURL()
}

func (c *TRClient) Login() bool {
	return Tr_Login()
}

// FetchTorrents 获取所有活动的种子列表.
func (c *TRClient) FetchTorrents() ([]*Torrent, error) {
	torrents := Tr_FetchTorrents()
	if torrents == nil {
		return nil, nil
	}
	var result []*Torrent
	for _, t := range torrents.Torrents {
		result = append(result, trTorrentToTorrent(t))
	}
	return result, nil
}

// FetchTorrentPeers 返回 Transmission 已内嵌在种子信息中的 Peer 列表.
func (c *TRClient) FetchTorrentPeers(torrent *Torrent) ([]*Peer, error) {
	return torrent.Peers, nil
}

func (c *TRClient) SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	return Tr_SubmitBlockPeer(blockPeerMap)
}

func (c *TRClient) SubmitShadowBanPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	return false // 不支持.
}

func trTorrentToTorrent(t Tr_TorrentStruct) *Torrent {
	var peers []*Peer
	var leecherCount int64
	for _, p := range t.Peers {
		if p.IsUploading {
			leecherCount++
		}
		peers = append(peers, &Peer{
			IP:       p.IP,
			Port:     p.Port,
			Client:   p.Client,
			DlSpeed:  p.DlSpeed,
			UpSpeed:  p.UpSpeed,
			Progress: p.Progress,
			// Transmission 这里不提供 PeerID 和上传/下载总量信息.
			Downloaded: -1,
			Uploaded:   -1,
		})
	}
	torrent := &Torrent{
		Hash:       t.InfoHash,
		TotalSize:  t.TotalSize,
		Tracker:    "",
		LeechCount: leecherCount,
		Peers:      peers,
	}
	if t.Private {
		torrent.Tracker = "Private"
	}
	return torrent
}

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

var Tr_csrfToken string
var Tr_csrfTokenMutex sync.RWMutex
var Tr_ipfilterStr = ""
var Tr_jsonHeader = map[string]string{"Content-Type": "application/json"}

func Tr_InitClient() {
	GoWithCrashLog("Tr_InitClient.StartServer", StartServer)
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
	if config.ClientURL == "" {
		return false
	}

	tr_SessionSetJSON, err := json.Marshal(Tr_RequestStruct{Method: "session-set", Args: Tr_SessionSetStruct{BlocklistEnabled: true, BlocklistURL: config.SyncServerURL + "/ipfilter.dat"}})
	if err != nil {
		Log("SetURL", GetLangText("Error-GenJSON"), true, err.Error())
		return false
	}

	Submit(config.ClientURL, tr_SessionSetJSON, false, true, &Tr_jsonHeader)

	return true
}
func (c *TRClient) Detect() bool {
	detectJSON, err := json.Marshal(Tr_RequestStruct{Method: "session-get", Args: Tr_ArgsStruct{Field: []string{"version"}}})
	if err != nil {
		Log("DetectVersion", GetLangText("Error-GenJSON"), true, err.Error())
		return false
	}

	detectStatusCode, _, _ := Submit(config.ClientURL, detectJSON, false, false, &Tr_jsonHeader)
	return (detectStatusCode == 200 || detectStatusCode == 409)
}
func Tr_Login() bool {
	loginJSON, err := json.Marshal(Tr_RequestStruct{Method: "session-get"})
	if err != nil {
		Log("Login", GetLangText("Error-GenJSON"), true, err.Error())
		return false
	}

	Submit(config.ClientURL, loginJSON, false, true, nil)

	Tr_csrfTokenMutex.RLock()
	token := Tr_csrfToken
	Tr_csrfTokenMutex.RUnlock()
	if token == "" {
		Log("Login", GetLangText("Error-Login"), true)
		return false
	}

	return true
}
func Tr_SetCSRFToken(csrfToken string) {
	Tr_csrfTokenMutex.Lock()
	Tr_csrfToken = csrfToken
	Tr_csrfTokenMutex.Unlock()
	Log("SetCSRFToken", GetLangText("Success-SetCSRFToken"), true, csrfToken)
}
func Tr_FetchTorrents() *Tr_TorrentsStruct {
	fetchJSON, err := json.Marshal(Tr_RequestStruct{Method: "torrent-get", Args: Tr_ArgsStruct{Field: []string{"hashString", "totalSize", "private", "peers"}}})
	if err != nil {
		Log("FetchTorrents", GetLangText("Error-GenJSON"), true, err.Error())
		return nil
	}

	_, _, fetchResponseBody := Submit(config.ClientURL, fetchJSON, true, true, &Tr_jsonHeader)
	if fetchResponseBody == nil {
		Log("FetchTorrents", GetLangText("Error-FetchResponse"), true)
		return nil
	}

	var fetchResponse Tr_TorrentsResponseStruct
	if err := json.Unmarshal(fetchResponseBody, &fetchResponse); err != nil {
		Log("FetchTorrents", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	return &fetchResponse.Args
}
func Tr_SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	ipfilterList := []string{}
	for peerIP := range blockPeerMap {
		ipfilterList = append(ipfilterList, ":" + peerIP + "-" + peerIP)
	}

	Tr_ipfilterStr = strings.Join(ipfilterList, "\n")

	tr_BlockListUpdateJSON, err := json.Marshal(Tr_RequestStruct{Method: "blocklist-update"})
	if err != nil {
		Log("SubmitBlockPeer", GetLangText("Error-GenJSON"), true, err.Error())
		return false
	}

	Submit(config.ClientURL, tr_BlockListUpdateJSON, true, true, &Tr_jsonHeader)

	return true
}
