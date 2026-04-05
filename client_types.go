package main

// Client 定义不同下载客户端的统一行为接口.
type Client interface {
	GetClientType() string
	ConfigPath() string
	SetURL() bool
	Login() bool
	FetchTorrents() ([]*Torrent, error)
	FetchTorrentPeers(torrent *Torrent) ([]*Peer, error)
	SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool
	SubmitShadowBanPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool
}

// Torrent 统一表示不同客户端返回的种子信息.
type Torrent struct {
	Hash       string
	Tracker    string
	LeechCount int64
	TotalSize  int64
	Peers      []*Peer
}

// Peer 统一表示不同客户端返回的 Peer 信息.
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
