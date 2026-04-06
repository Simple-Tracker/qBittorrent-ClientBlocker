package main

import (
	"bytes"
	"encoding/json"
	"github.com/PuerkitoBio/goquery"
	"strconv"
	"strings"
)

// BCClient 实现了 BitComet 的客户端接口.
type BCClient struct {
	Version int // 1: HTML, 2: JSON (v2.09+)
}

func (c *BCClient) GetClientType() string {
	return "BitComet"
}

func (c *BCClient) ConfigPath() string {
	return ""
}

func (c *BCClient) SetURL() bool {
	return false
}

func (c *BCClient) Login() bool {
	return BC_Login()
}

// FetchTorrents 获取所有活动的种子列表.
func (c *BCClient) FetchTorrents() ([]*Torrent, error) {
	if c.Version == 2 {
		return c.FetchTorrents_v2()
	}

	torrents := BC_FetchTorrents()
	if torrents == nil {
		return nil, nil
	}
	var result []*Torrent
	for id, t := range *torrents {
		result = append(result, &Torrent{
			Hash:       strconv.Itoa(id),
			TotalSize:  t.TotalSize,
			Tracker:    "Unsupported",
			LeechCount: 233,
		})
	}
	return result, nil
}

// FetchTorrentPeers 获取特定种子的 Peer 列表.
func (c *BCClient) FetchTorrentPeers(torrent *Torrent) ([]*Peer, error) {
	if c.Version == 2 {
		return c.FetchTorrentPeers_v2(torrent)
	}

	peers := BC_FetchTorrentPeers(torrent.Hash)
	if peers == nil {
		return nil, nil
	}
	var result []*Peer
	for _, p := range *peers {
		result = append(result, &Peer{
			IP:         p.IP,
			Port:       p.Port,
			Client:     p.Client,
			DlSpeed:    p.DlSpeed,
			UpSpeed:    p.UpSpeed,
			Progress:   p.Progress,
			Downloaded: p.Downloaded,
			Uploaded:   p.Uploaded,
			ID:         "", // BitComet 不提供 PeerID.
		})
	}
	return result, nil
}

func (c *BCClient) SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	if c.Version == 2 {
		return c.BC_SubmitBlockPeer_v2(blockPeerMap)
	}
	return false // BitComet 1.x 暂未通过 WebUI 实现封禁.
}

// Version 2 实现逻辑.
func (c *BCClient) FetchTorrents_v2() ([]*Torrent, error) {
	_, _, responseBody := Fetch(config.ClientURL+"/api_v2/task_list/get?state_group=ACTIVE", true, true, false, nil)
	if responseBody == nil {
		return nil, nil
	}

	var resp BC_v2_TaskListResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		Log("FetchTorrents_v2", GetLangText("Error-Parse"), true, err.Error())
		return nil, err
	}

	var result []*Torrent
	for _, t := range resp.TaskList {
		if strings.ToUpper(t.Type) != "BT" {
			continue
		}
		result = append(result, &Torrent{
			Hash:       t.TaskID,
			TotalSize:  t.Size,
			Tracker:    "BitComet-v2",
			LeechCount: int64(t.LeechCount),
		})
	}
	return result, nil
}

func (c *BCClient) FetchTorrentPeers_v2(torrent *Torrent) ([]*Peer, error) {
	_, _, responseBody := Fetch(config.ClientURL+"/api/task/peers/get?task_id="+torrent.Hash+"&groups=peers_connected", true, true, false, nil)
	if responseBody == nil {
		return nil, nil
	}

	var resp BC_v2_PeerListResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		Log("FetchTorrentPeers_v2", GetLangText("Error-Parse"), true, err.Error())
		return nil, err
	}

	var result []*Peer
	for _, p := range resp.PeerList {
		result = append(result, &Peer{
			IP:         p.IP,
			Port:       p.Port,
			Client:     p.Client,
			DlSpeed:    p.DlSpeed,
			UpSpeed:    p.UpSpeed,
			Progress:   p.Progress / 100.0, // API 返回通常是 0-100.
			Downloaded: p.Downloaded,
			Uploaded:   p.Uploaded,
		})
	}
	return result, nil
}

func (c *BCClient) BC_SubmitBlockPeer_v2(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	// 按 Torrent 分组 IP 以匹配 ban_ip 接口要求.
	taskIPs := make(map[string][]string)
	for peerIP, peerInfo := range blockPeerMap {
		if peerInfo.InfoHash != "" {
			taskIPs[peerInfo.InfoHash] = append(taskIPs[peerInfo.InfoHash], peerIP)
		}
	}

	if len(taskIPs) == 0 {
		return true
	}

	allSuccess := true
	for taskID, ips := range taskIPs {
		params := BC_v2_BanParams{
			TaskID:  taskID,
			BanTime: "ban_ip_forever",
			IPList:  ips,
		}
		postData, _ := json.Marshal(params)
		code, _, _ := Submit(config.ClientURL+"/api/task/peers/ban_ip", postData, true, true, &Tr_jsonHeader)
		if code != 200 {
			allSuccess = false
		}
	}
	return allSuccess
}

type BC_v2_BanParams struct {
	TaskID  string   `json:"task_id"`
	BanTime string   `json:"ban_time"`
	IPList  []string `json:"ip_list"`
}

func (c *BCClient) SubmitShadowBanPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	return false // 不支持.
}

type BC_TorrentStruct struct {
	TotalSize int64
	UpSpeed   int64
}
type BC_PeerStruct struct {
	IP     string
	Port   int
	Client string
	//	PeerID     string
	Progress   float64
	Downloaded int64
	Uploaded   int64
	DlSpeed    int64
	UpSpeed    int64
}

// BitComet v2 JSON API 结构体.
type BC_v2_CommonResponse struct {
	Result string `json:"result"`
}
type BC_v2_TaskListResponse struct {
	TaskList []BC_v2_Task `json:"movie_list"` // 该 API 实际返回的是 movie_list.
}
type BC_v2_Task struct {
	TaskID    string `json:"task_id"`
	Type      string `json:"type"`
	Size      int64  `json:"total_size"`
	UpSpeed   int64  `json:"upload_speed"`
	Status    string `json:"state"`
	InfoHash  string `json:"info_hash"`
	Name      string `json:"name"`
	Progress  int    `json:"progress"`
	LeechCount int    `json:"leechers_count"`
}
type BC_v2_PeerListResponse struct {
	PeerList []BC_v2_Peer `json:"peers_connected"`
}
type BC_v2_Peer struct {
	IP         string  `json:"address"`
	Port       int     `json:"remoteport"`
	Client     string  `json:"clienttype"`
	Progress   float64 `json:"progress"`
	DlSpeed    int64   `json:"downrate"`
	UpSpeed    int64   `json:"uprate"`
	Downloaded int64   `json:"downsize"`
	Uploaded   int64   `json:"upsize"`
}

func BC_ParseTorrentLink(torrentLinkStr string) int {
	torrentIDSplit1 := strings.SplitN(StrTrim(torrentLinkStr), "?id=", 2)
	if len(torrentIDSplit1) < 2 {
		return -2
	}

	torrentIDStr := strings.SplitN(torrentIDSplit1[1], "&", 2)[0]
	torrentID, err := strconv.Atoi(torrentIDStr)
	if err != nil {
		return -3
	}

	return torrentID
}
func BC_ParseSize(sizeStr string) int64 {
	sizeStr = StrTrim(sizeStr)
	if sizeStr == "" {
		return 0
	}

	sizeStrSplit := strings.SplitN(sizeStr, " ", 2)
	if len(sizeStrSplit) < 2 || len(sizeStrSplit[1]) < 2 {
		return -1
	}

	rawSize, err := strconv.ParseFloat(sizeStrSplit[0], 64)
	if err != nil {
		return -2
	}

	matched := false
	multipler := 1
	switch strings.ToUpper(sizeStrSplit[1]) {
	case "EB":
		multipler *= 1024
		fallthrough
	case "PB":
		multipler *= 1024
		fallthrough
	case "TB":
		multipler *= 1024
		fallthrough
	case "GB":
		multipler *= 1024
		fallthrough
	case "MB":
		multipler *= 1024
		fallthrough
	case "KB":
		multipler *= 1024
		fallthrough
	case "B":
		matched = true
	}

	if !matched {
		return -3
	}

	return int64(rawSize * float64(multipler))
}
func BC_ParseSpeed(speedStr string) int64 {
	speedStr = StrTrim(speedStr)
	if speedStr == "" {
		return 0
	}

	speedStrSplit := strings.SplitN(speedStr, "/", 2)

	if len(speedStrSplit) < 2 || len(speedStrSplit[1]) != 1 {
		return -1
	}

	return BC_ParseSize(speedStrSplit[0])
}
func BC_ParsePercent(percentStr string) float64 {
	percentStr = StrTrim(percentStr)
	if len(percentStr) < 2 {
		return -1
	}

	percentStr = percentStr[:(len(percentStr) - 1)]
	percent, err := strconv.ParseFloat(percentStr, 64)
	if err != nil {
		return -2
	}

	return percent
}
func BC_ParseIP(ipStr string) (string, int) {
	ipStr = strings.ToLower(StrTrim(ipStr))
	if ipStr == "myself" {
		return "", -1
	}

	lastColonIndex := strings.LastIndex(ipStr, ":")
	if lastColonIndex == -1 || len(ipStr) < (lastColonIndex+2) {
		return "", -2
	}

	ipWithoutPortStr := ipStr[:lastColonIndex]
	portStr := ipStr[(lastColonIndex + 1):]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", -3
	}

	return ipWithoutPortStr, port
}
func (c *BCClient) Detect() bool {
	// 优先探测 Version 2 (JSON API).
	apiResponseStatusCode, _, _ := Fetch(config.ClientURL+"/api_v2/task_list/get", false, false, false, nil)
	if apiResponseStatusCode == 200 || apiResponseStatusCode == 401 {
		c.Version = 2
		Log("DetectClient", "BitComet (Version 2 - JSON API) Detected", true)
		return true
	}

	// 回落探测 Version 1 (HTML).
	apiResponseStatusCode, apiResponseHeaders, _ := Fetch(config.ClientURL+"/panel/", false, false, false, nil)
	if apiResponseStatusCode == 401 && strings.Contains(apiResponseHeaders.Get("WWW-Authenticate"), "BitComet") {
		c.Version = 1
		Log("DetectClient", "BitComet (Version 1 - HTML) Detected", true)
		return true
	}

	return false
}
func BC_Login() bool {
	apiResponseStatusCode, _, _ := Fetch(config.ClientURL+"/panel/", false, true, false, nil)
	return (apiResponseStatusCode == 200)
}
func BC_FetchTorrents() *map[int]BC_TorrentStruct {
	_, _, torrentsResponseBody := Fetch(config.ClientURL+"/panel/task_list?group=active", true, true, false, nil)
	if torrentsResponseBody == nil {
		Log("FetchTorrents", GetLangText("Error"), true)
		return nil
	}

	document, err := goquery.NewDocumentFromReader(bytes.NewReader(torrentsResponseBody))
	if err != nil {
		Log("FetchTorrents", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	torrentsMap := make(map[int]BC_TorrentStruct)
	document.Find("table").Last().Find("tbody > tr").Each(func(index int, element *goquery.Selection) {
		if index == 0 {
			return
		}

		torrentStatus := ""
		torrentID := 0
		var torrentSize int64 = -233
		var torrentUpSpeed int64 = -233
		element.Find("td").EachWithBreak(func(tdIndex int, tdElement *goquery.Selection) bool {
			switch tdIndex {
			case 0:
				if strings.ToUpper(StrTrim(tdElement.Text())) != "BT" {
					return false
				}
			case 1:
				href, exists := tdElement.Find("a").Attr("href")
				if !exists {
					return false
				}

				torrentID = BC_ParseTorrentLink(href)
			case 2:
				torrentStatus = strings.ToLower(StrTrim(tdElement.Text()))
			case 4:
				torrentSize = BC_ParseSize(tdElement.Text())
			case 7:
				torrentUpSpeed = BC_ParseSpeed(tdElement.Text())
			}

			return true
		})

		if torrentStatus == "" || torrentID <= 0 || torrentSize <= 0 || torrentUpSpeed < 0 {
			return
		}

		torrentsMap[torrentID] = BC_TorrentStruct{TotalSize: torrentSize, UpSpeed: torrentUpSpeed}
	})

	return &torrentsMap
}
func BC_FetchTorrentPeers(infoHash string) *[]BC_PeerStruct {
	_, _, torrentPeersResponseBody := Fetch(config.ClientURL+"/panel/task_detail?id="+infoHash+"&show=peers", true, true, false, nil)
	if torrentPeersResponseBody == nil {
		Log("FetchTorrentPeers", GetLangText("Error"), true)
		return nil
	}

	document, err := goquery.NewDocumentFromReader(bytes.NewReader(torrentPeersResponseBody))
	if err != nil {
		Log("FetchTorrentPeers", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	torrentPeersMap := []BC_PeerStruct{}
	document.Find("table").Last().Find("tbody > tr").Each(func(index int, element *goquery.Selection) {
		if index == 0 {
			return
		}

		peerIP := ""
		peerPort := -233
		var peerProgress float64 = -233
		var peerDlSpeed int64 = -233
		var peerUpSpeed int64 = -233
		var peerDownloaded int64 = -233
		var peerUploaded int64 = -233
		peerClient := ""
		element.Find("td").EachWithBreak(func(tdIndex int, tdElement *goquery.Selection) bool {
			switch tdIndex {
			case 0:
				peerIP, peerPort = BC_ParseIP(tdElement.Text())
			case 1:
				peerProgress = BC_ParsePercent(tdElement.Text())
			case 2:
				peerDlSpeed = BC_ParseSpeed(tdElement.Text())
			case 3:
				peerUpSpeed = BC_ParseSpeed(tdElement.Text())
			case 4:
				peerDownloaded = BC_ParseSize(tdElement.Text())
			case 5:
				peerUploaded = BC_ParseSize(tdElement.Text())
			case 9:
				peerClient = tdElement.Text()
			}

			return true
		})

		if peerIP == "" || peerPort < 0 || peerProgress < 0 || peerDlSpeed < 0 || peerUpSpeed < 0 || peerDownloaded < 0 || peerUploaded < 0 {
			return
		}

		peerStruct := BC_PeerStruct{IP: peerIP, Port: peerPort, Client: peerClient, Progress: peerProgress, Downloaded: peerDownloaded, Uploaded: peerUploaded, DlSpeed: peerDlSpeed, UpSpeed: peerUpSpeed}
		torrentPeersMap = append(torrentPeersMap, peerStruct)
	})

	return &torrentPeersMap
}
