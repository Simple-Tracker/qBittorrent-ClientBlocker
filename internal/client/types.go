package client

import "net/http"

// Client 定义扫描器与下载客户端之间的协议接口.
type Client interface {
	GetClientType() string
	ConfigPath() string
	SetURL() bool
	Login() bool
	FetchTorrents() ([]*Torrent, error)
	FetchTorrentPeers(*Torrent) ([]*Peer, error)
	SubmitBlockPeer(map[string]BanTarget) bool
	SubmitShadowBanPeer(map[string]BanTarget) bool
	Detect() bool
}

type Torrent struct {
	Hash       string
	Tracker    string
	LeechCount int64
	TotalSize  int64
	Peers      []*Peer
}

type Peer struct {
	IP         string
	Port       int
	ID         string
	Client     string
	DlSpeed    int64
	UpSpeed    int64
	Progress   float64
	Downloaded int64
	Uploaded   int64
}

// BanTarget 仅包含向下载客户端提交封禁所需的信息.
type BanTarget struct {
	Ports   map[int]bool
	TaskIDs []string
}

type SessionClient interface {
	SessionToken() string
	SetSessionToken(string)
}

type BlocklistHandler interface {
	ServeBlocklist(http.ResponseWriter, *http.Request) bool
}

type ShadowBanTester interface{ TestShadowBanAPI() bool }
