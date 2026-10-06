package transmission

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

// Client 实现了 Transmission 的客户端接口.
type Client struct {
	services       client.Services
	csrfToken      string
	csrfTokenMutex sync.RWMutex
	ipfilter       string
	filterMutex    sync.RWMutex
}

func New(services client.Services) *Client { return &Client{services: services.WithDefaults()} }

func (c *Client) GetClientType() string {
	return "Transmission"
}

func (c *Client) ConfigPath() string {
	return ""
}

func (c *Client) SetURL() bool {
	return c.setURL()
}

func (c *Client) Login() bool {
	return c.login()
}

// FetchTorrents 获取所有活动的种子列表.
func (c *Client) FetchTorrents() ([]*client.Torrent, error) {
	torrents := c.fetchTorrents()
	if torrents == nil {
		return nil, nil
	}
	result := []*client.Torrent{}
	for _, t := range torrents.Torrents {
		result = append(result, TorrentFromResponse(t))
	}
	return result, nil
}

// FetchTorrentPeers 返回 Transmission 已内嵌在种子信息中的 client.Peer 列表.
func (c *Client) FetchTorrentPeers(torrent *client.Torrent) ([]*client.Peer, error) {
	if torrent.Peers == nil {
		return []*client.Peer{}, nil
	}
	return torrent.Peers, nil
}

func (c *Client) SubmitBlockPeer(blockPeerMap map[string]client.BanTarget) bool {
	return c.submitBlockPeer(blockPeerMap)
}

func (c *Client) SubmitShadowBanPeer(blockPeerMap map[string]client.BanTarget) bool {
	return false // 不支持.
}

func TorrentFromResponse(t TorrentRecord) *client.Torrent {
	peers := []*client.Peer{}
	var leecherCount int64
	for _, p := range t.Peers {
		if p.IsUploading {
			leecherCount++
		}
		peers = append(peers, &client.Peer{
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
	torrent := &client.Torrent{
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

type Request struct {
	Method string      `json:"method"`
	Args   interface{} `json:"arguments"`
}
type Response struct {
	Result string `json:"result"`
}
type TorrentsResponse struct {
	Result string   `json:"result"`
	Args   Torrents `json:"arguments"`
}
type Args struct {
	Field []string `json:"fields"`
}
type TorrentArgs struct {
	IDs   []string `json:"ids"`
	Field []string `json:"fields"`
}
type SessionSettings struct {
	BlocklistEnabled bool   `json:"blocklist-enabled"`
	BlocklistSize    int    `json:"blocklist-size"`
	BlocklistURL     string `json:"blocklist-url"`
}
type Torrents struct {
	Torrents []TorrentRecord `json:"torrents"`
}
type TorrentRecord struct {
	InfoHash  string       `json:"hashString"`
	TotalSize int64        `json:"totalSize"`
	Private   bool         `json:"private"`
	Peers     []PeerRecord `json:"peers"`
}
type PeerRecord struct {
	IP          string  `json:"address"`
	Port        int     `json:"port"`
	Client      string  `json:"clientName"`
	Progress    float64 `json:"progress"`
	IsUploading bool    `json:"isUploadingTo"`
	DlSpeed     int64   `json:"rateToClient"`
	UpSpeed     int64   `json:"rateToPeer"`
}

var jsonHeader = map[string]string{"Content-Type": "application/json"}

func (c *Client) ServeBlocklist(w http.ResponseWriter, r *http.Request) bool {
	if strings.SplitN(r.RequestURI, "?", 2)[0] == "/ipfilter.dat" {
		w.WriteHeader(200)
		c.filterMutex.RLock()
		filter := c.ipfilter
		c.filterMutex.RUnlock()
		w.Write([]byte(filter))

		return true
	}

	return false
}
func (c *Client) setURL() bool {
	if c.services.Snapshot().URL == "" {
		return false
	}

	tr_SessionSetJSON, err := json.Marshal(Request{Method: "session-set", Args: SessionSettings{BlocklistEnabled: true, BlocklistURL: c.services.Snapshot().BlocklistURL}})
	if err != nil {
		c.services.LogError("SetURL", c.services.Text("Error-GenJSON"), true, err.Error())
		return false
	}

	c.services.Submit(c.services.Snapshot().URL, tr_SessionSetJSON, false, true, &jsonHeader)

	return true
}
func (c *Client) Detect() bool {
	detectJSON, err := json.Marshal(Request{Method: "session-get", Args: Args{Field: []string{"version"}}})
	if err != nil {
		c.services.LogError("DetectVersion", c.services.Text("Error-GenJSON"), true, err.Error())
		return false
	}

	detectStatusCode, _, _ := c.services.Submit(c.services.Snapshot().URL, detectJSON, false, false, &jsonHeader)
	return (detectStatusCode == 200 || detectStatusCode == 409)
}
func (c *Client) login() bool {
	loginJSON, err := json.Marshal(Request{Method: "session-get"})
	if err != nil {
		c.services.LogError("Login", c.services.Text("Error-GenJSON"), true, err.Error())
		return false
	}

	c.services.Submit(c.services.Snapshot().URL, loginJSON, false, true, nil)

	c.csrfTokenMutex.RLock()
	token := c.csrfToken
	c.csrfTokenMutex.RUnlock()
	if token == "" {
		c.services.LogError("Login", c.services.Text("Error-Login"), true)
		return false
	}

	return true
}
func (c *Client) SetSessionToken(csrfToken string) {
	c.csrfTokenMutex.Lock()
	c.csrfToken = csrfToken
	c.csrfTokenMutex.Unlock()
	c.services.Log("SetCSRFToken", c.services.Text("Success-SetCSRFToken"), true, csrfToken)
}
func (c *Client) fetchTorrents() *Torrents {
	fetchJSON, err := json.Marshal(Request{Method: "torrent-get", Args: Args{Field: []string{"hashString", "totalSize", "private", "peers"}}})
	if err != nil {
		c.services.LogError("FetchTorrents", c.services.Text("Error-GenJSON"), true, err.Error())
		return nil
	}

	_, _, fetchResponseBody := c.services.Submit(c.services.Snapshot().URL, fetchJSON, true, true, &jsonHeader)
	if fetchResponseBody == nil {
		c.services.LogError("FetchTorrents", c.services.Text("Error-FetchResponse"), true)
		return nil
	}

	var fetchResponse TorrentsResponse
	if err := json.Unmarshal(fetchResponseBody, &fetchResponse); err != nil {
		c.services.LogError("FetchTorrents", c.services.Text("Error-Parse"), true, err.Error())
		return nil
	}

	return &fetchResponse.Args
}
func (c *Client) submitBlockPeer(blockPeerMap map[string]client.BanTarget) bool {
	ipfilterList := []string{}
	for peerIP := range blockPeerMap {
		ipfilterList = append(ipfilterList, peerIP)
	}

	c.filterMutex.Lock()
	c.ipfilter = strings.Join(ipfilterList, "\n")
	c.filterMutex.Unlock()

	tr_BlockListUpdateJSON, err := json.Marshal(Request{Method: "blocklist-update"})
	if err != nil {
		c.services.LogError("SubmitBlockPeer", c.services.Text("Error-GenJSON"), true, err.Error())
		return false
	}

	c.services.Submit(c.services.Snapshot().URL, tr_BlockListUpdateJSON, true, true, &jsonHeader)

	return true
}

func (c *Client) SessionToken() string {
	c.csrfTokenMutex.RLock()
	defer c.csrfTokenMutex.RUnlock()
	return c.csrfToken
}
