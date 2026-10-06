package bitcomet

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

// Client 实现了 BitComet 的客户端接口.
type Client struct {
	services      client.Services
	Version       int // 1: HTML, 2: JSON (v2.09+)
	banURL        string
	bannedTaskIPs map[string]map[string]bool
}

func (c *Client) GetClientType() string {
	return "BitComet"
}

func (c *Client) ConfigPath() string {
	return ""
}

func (c *Client) SetURL() bool {
	return false
}

func (c *Client) Login() bool {
	return c.login()
}

// FetchTorrents 获取所有活动的种子列表.
func (c *Client) FetchTorrents() ([]*client.Torrent, error) {
	if c.Version == 2 {
		return c.FetchTorrents_v2()
	}

	torrents := c.fetchTorrents()
	if torrents == nil {
		return nil, nil
	}
	result := []*client.Torrent{}
	for id, t := range *torrents {
		result = append(result, &client.Torrent{
			Hash:       strconv.Itoa(id),
			TotalSize:  t.TotalSize,
			Tracker:    "Unsupported",
			LeechCount: 233,
		})
	}
	return result, nil
}

// FetchTorrentPeers 获取特定种子的 client.Peer 列表.
func (c *Client) FetchTorrentPeers(torrent *client.Torrent) ([]*client.Peer, error) {
	if c.Version == 2 {
		return c.FetchTorrentPeers_v2(torrent)
	}

	peers := c.fetchTorrentPeers(torrent.Hash)
	if peers == nil {
		return nil, nil
	}
	result := []*client.Peer{}
	for _, p := range *peers {
		result = append(result, &client.Peer{
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

func (c *Client) SubmitBlockPeer(blockPeerMap map[string]client.BanTarget) bool {
	if c.Version == 2 {
		return c.submitBlockPeerV2(blockPeerMap)
	}
	return false // BitComet 1.x 暂未通过 WebUI 实现封禁.
}

// 版本 2 的实现逻辑.
func (c *Client) FetchTorrents_v2() ([]*client.Torrent, error) {
	_, _, responseBody := c.services.Fetch(c.services.Snapshot().URL+"/api_v2/task_list/get?state_group=ACTIVE", true, true, false, nil)
	if responseBody == nil {
		return nil, nil
	}

	var resp TaskListResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		c.services.LogError("FetchTorrents_v2", c.services.Text("Error-Parse"), true, err.Error())
		return nil, err
	}

	result := []*client.Torrent{}
	for _, t := range resp.TaskList {
		if strings.ToUpper(t.Type) != "BT" {
			continue
		}
		result = append(result, &client.Torrent{
			Hash:       t.TaskID,
			TotalSize:  t.Size,
			Tracker:    "BitComet-v2",
			LeechCount: int64(t.LeechCount),
		})
	}
	return result, nil
}

func (c *Client) FetchTorrentPeers_v2(torrent *client.Torrent) ([]*client.Peer, error) {
	_, _, responseBody := c.services.Fetch(c.services.Snapshot().URL+"/api/task/peers/get?task_id="+torrent.Hash+"&groups=peers_connected", true, true, false, nil)
	if responseBody == nil {
		return nil, nil
	}

	var resp PeerListResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		c.services.LogError("FetchTorrentPeers_v2", c.services.Text("Error-Parse"), true, err.Error())
		return nil, err
	}

	result := []*client.Peer{}
	for _, p := range resp.PeerList {
		result = append(result, &client.Peer{
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

func (c *Client) submitBlockPeerV2(blockPeerMap map[string]client.BanTarget) bool {
	clientURL := c.services.Snapshot().URL
	if c.banURL != clientURL || c.bannedTaskIPs == nil {
		c.banURL = clientURL
		c.bannedTaskIPs = make(map[string]map[string]bool)
	}

	allSuccess := true
	for taskID, bannedIPs := range c.bannedTaskIPs {
		var removed []string
		for ip := range bannedIPs {
			if _, retained := blockPeerMap[ip]; !retained {
				removed = append(removed, ip)
			}
		}
		if len(removed) == 0 {
			continue
		}
		// 仅解除本实例成功提交的地址, 失败时保留记录供下一轮重试.
		params := UnbanParams{TaskID: taskID, UnbanRange: "unban_peers", IPList: removed}
		if !c.submitPeerAction(clientURL, "unban_peers", params) {
			allSuccess = false
			continue
		}
		for _, ip := range removed {
			delete(bannedIPs, ip)
		}
		if len(bannedIPs) == 0 {
			delete(c.bannedTaskIPs, taskID)
		}
	}

	// 按 client.Torrent 分组 IP 以匹配 ban_ip 接口要求.
	taskIPs := make(map[string][]string)
	for peerIP, peerInfo := range blockPeerMap {
		taskIDs := make(map[string]bool)
		for _, taskID := range peerInfo.TaskIDs {
			if taskID != "" {
				taskIDs[taskID] = true
			}
		}
		for taskID := range taskIDs {
			if !c.bannedTaskIPs[taskID][peerIP] {
				taskIPs[taskID] = append(taskIPs[taskID], peerIP)
			}
		}
	}

	for taskID, ips := range taskIPs {
		params := BanParams{
			TaskID:  taskID,
			BanTime: "ban_ip_forever",
			IPList:  ips,
		}
		if !c.submitPeerAction(clientURL, "ban_ip", params) {
			allSuccess = false
			continue
		}
		if c.bannedTaskIPs[taskID] == nil {
			c.bannedTaskIPs[taskID] = make(map[string]bool)
		}
		for _, ip := range ips {
			c.bannedTaskIPs[taskID][ip] = true
		}
	}
	return allSuccess
}

// API: https://wiki-zh.bitcomet.com/webui_api调用接口/task-details/
func (c *Client) submitPeerAction(clientURL, action string, params any) bool {
	postData, err := json.Marshal(params)
	if err != nil {
		return false
	}
	code, _, body := c.services.Submit(clientURL+"/api/task/peers/"+action, postData, true, true, &jsonHeader)
	var response CommonResponse
	if code != 200 || json.Unmarshal(body, &response) != nil {
		return false
	}
	if response.Result != "" && !strings.EqualFold(response.Result, "success") && !strings.EqualFold(response.Result, "ok") {
		return false
	}
	if response.ErrorCode != "" && !strings.EqualFold(response.ErrorCode, "ok") {
		return false
	}
	return response.Result != "" || response.ErrorCode != ""
}

type BanParams struct {
	TaskID  string   `json:"task_id"`
	BanTime string   `json:"ban_time"`
	IPList  []string `json:"ip_list"`
}

type UnbanParams struct {
	TaskID     string   `json:"task_id"`
	UnbanRange string   `json:"unban_range"`
	IPList     []string `json:"ip_list"`
}

func (c *Client) SubmitShadowBanPeer(blockPeerMap map[string]client.BanTarget) bool {
	return false // 不支持.
}

type TorrentRecord struct {
	TotalSize int64
	UpSpeed   int64
}
type PeerRecord struct {
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
type CommonResponse struct {
	Result    string `json:"result"`
	ErrorCode string `json:"error_code"`
}
type TaskListResponse struct {
	TaskList []TaskRecord `json:"movie_list"` // 该 API 实际返回的是 movie_list.
}
type TaskRecord struct {
	TaskID     string `json:"task_id"`
	Type       string `json:"type"`
	Size       int64  `json:"total_size"`
	UpSpeed    int64  `json:"upload_speed"`
	Status     string `json:"state"`
	InfoHash   string `json:"info_hash"`
	Name       string `json:"name"`
	Progress   int    `json:"progress"`
	LeechCount int    `json:"leechers_count"`
}
type PeerListResponse struct {
	PeerList []PeerV2Record `json:"peers_connected"`
}
type PeerV2Record struct {
	IP         string  `json:"address"`
	Port       int     `json:"remoteport"`
	Client     string  `json:"clienttype"`
	Progress   float64 `json:"progress"`
	DlSpeed    int64   `json:"downrate"`
	UpSpeed    int64   `json:"uprate"`
	Downloaded int64   `json:"downsize"`
	Uploaded   int64   `json:"upsize"`
}

func ParseTorrentLink(torrentLinkStr string) int {
	torrentIDSplit1 := strings.SplitN(client.Trim(torrentLinkStr), "?id=", 2)
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
func ParseSize(sizeStr string) int64 {
	sizeStr = client.Trim(sizeStr)
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
func ParseSpeed(speedStr string) int64 {
	speedStr = client.Trim(speedStr)
	if speedStr == "" {
		return 0
	}

	speedStrSplit := strings.SplitN(speedStr, "/", 2)

	if len(speedStrSplit) < 2 || len(speedStrSplit[1]) != 1 {
		return -1
	}

	return ParseSize(speedStrSplit[0])
}
func ParsePercent(percentStr string) float64 {
	percentStr = client.Trim(percentStr)
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
func ParseIP(ipStr string) (string, int) {
	ipStr = strings.ToLower(client.Trim(ipStr))
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
func (c *Client) Detect() bool {
	// 优先探测版本 2 (JSON API).
	apiResponseStatusCode, _, _ := c.services.Fetch(c.services.Snapshot().URL+"/api_v2/task_list/get", false, false, false, nil)
	if apiResponseStatusCode == 200 || apiResponseStatusCode == 401 {
		c.Version = 2
		c.services.Log("DetectClient", "BitComet (Version 2 - JSON API) Detected", true)
		return true
	}

	// 回退探测版本 1 (HTML).
	apiResponseStatusCode, apiResponseHeaders, _ := c.services.Fetch(c.services.Snapshot().URL+"/panel/", false, false, false, nil)
	if apiResponseStatusCode == 401 && strings.Contains(apiResponseHeaders.Get("WWW-Authenticate"), "BitComet") {
		c.Version = 1
		c.services.Log("DetectClient", "BitComet (Version 1 - HTML) Detected", true)
		return true
	}

	return false
}
func (c *Client) login() bool {
	apiResponseStatusCode, _, _ := c.services.Fetch(c.services.Snapshot().URL+"/panel/", false, true, false, nil)
	return (apiResponseStatusCode == 200)
}
func (c *Client) fetchTorrents() *map[int]TorrentRecord {
	_, _, torrentsResponseBody := c.services.Fetch(c.services.Snapshot().URL+"/panel/task_list?group=active", true, true, false, nil)
	if torrentsResponseBody == nil {
		c.services.LogError("FetchTorrents", c.services.Text("Error"), true)
		return nil
	}

	document, err := goquery.NewDocumentFromReader(bytes.NewReader(torrentsResponseBody))
	if err != nil {
		c.services.LogError("FetchTorrents", c.services.Text("Error-Parse"), true, err.Error())
		return nil
	}

	torrentsMap := make(map[int]TorrentRecord)
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
				if strings.ToUpper(client.Trim(tdElement.Text())) != "BT" {
					return false
				}
			case 1:
				href, exists := tdElement.Find("a").Attr("href")
				if !exists {
					return false
				}

				torrentID = ParseTorrentLink(href)
			case 2:
				torrentStatus = strings.ToLower(client.Trim(tdElement.Text()))
			case 4:
				torrentSize = ParseSize(tdElement.Text())
			case 7:
				torrentUpSpeed = ParseSpeed(tdElement.Text())
			}

			return true
		})

		if torrentStatus == "" || torrentID <= 0 || torrentSize <= 0 || torrentUpSpeed < 0 {
			return
		}

		torrentsMap[torrentID] = TorrentRecord{TotalSize: torrentSize, UpSpeed: torrentUpSpeed}
	})

	return &torrentsMap
}
func (c *Client) fetchTorrentPeers(infoHash string) *[]PeerRecord {
	_, _, torrentPeersResponseBody := c.services.Fetch(c.services.Snapshot().URL+"/panel/task_detail?id="+infoHash+"&show=peers", true, true, false, nil)
	if torrentPeersResponseBody == nil {
		c.services.LogError("FetchTorrentPeers", c.services.Text("Error"), true)
		return nil
	}

	document, err := goquery.NewDocumentFromReader(bytes.NewReader(torrentPeersResponseBody))
	if err != nil {
		c.services.LogError("FetchTorrentPeers", c.services.Text("Error-Parse"), true, err.Error())
		return nil
	}

	torrentPeersMap := []PeerRecord{}
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
				peerIP, peerPort = ParseIP(tdElement.Text())
			case 1:
				peerProgress = ParsePercent(tdElement.Text())
			case 2:
				peerDlSpeed = ParseSpeed(tdElement.Text())
			case 3:
				peerUpSpeed = ParseSpeed(tdElement.Text())
			case 4:
				peerDownloaded = ParseSize(tdElement.Text())
			case 5:
				peerUploaded = ParseSize(tdElement.Text())
			case 9:
				peerClient = tdElement.Text()
			}

			return true
		})

		if peerIP == "" || peerPort < 0 || peerProgress < 0 || peerDlSpeed < 0 || peerUpSpeed < 0 || peerDownloaded < 0 || peerUploaded < 0 {
			return
		}

		peerStruct := PeerRecord{IP: peerIP, Port: peerPort, Client: peerClient, Progress: peerProgress, Downloaded: peerDownloaded, Uploaded: peerUploaded, DlSpeed: peerDlSpeed, UpSpeed: peerUpSpeed}
		torrentPeersMap = append(torrentPeersMap, peerStruct)
	})

	return &torrentPeersMap
}

var jsonHeader = map[string]string{"Content-Type": "application/json"}

func New(services client.Services) *Client { return &Client{services: services.WithDefaults()} }
