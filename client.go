package main

import "sync"

// currentClient 是当前正在使用的客户端实例.
var currentClient Client

// currentClientType 是当前客户端的类型名称, 如 "qBittorrent".
var currentClientType = ""
var clientTypeMutex sync.RWMutex

func CurrentClientTypeSnapshot() string {
	clientTypeMutex.RLock()
	defer clientTypeMutex.RUnlock()
	return currentClientType
}

func SetCurrentClientType(name string) {
	clientTypeMutex.Lock()
	currentClientType = name
	clientTypeMutex.Unlock()
}

// IsBanPort 返回当前客户端是否支持按端口封禁.
func IsBanPort() bool {
	if currentClientType == "qBittorrent" && qB_useNewBanPeersMethod {
		return true
	}

	return false
}

// IsSupportClient 返回当前是否已检测到支持的客户端.
func IsSupportClient() bool {
	return currentClient != nil
}

// InitClient 初始化客户端特定功能.
func InitClient() {
	if currentClientType == "Transmission" {
		Tr_InitClient()
	}
}

// SetURLFromClient 尝试从本地配置文件读取并设置客户端 API 地址.
func SetURLFromClient() {
	if ConfigSnapshot().ClientURL == "" {
		qb := &QBClient{}
		if !qb.SetURL() {
			tr := &TRClient{}
			tr.SetURL()
		}
	}
}

// DetectClient 自动探测当前使用的下载软件类型.
func DetectClient() bool {
	// 先尝试 qBittorrent.
	qb := &QBClient{}
	if ConfigSnapshot().ClientType == "" || ConfigSnapshot().ClientType == qb.GetClientType() {
		if qb.Detect() {
			currentClient = qb
			SetCurrentClientType(qb.GetClientType())
			Log("DetectClient", GetLangText("Success-DetectClient"), true, currentClientType)
			return true
		}
	}

	// 再尝试 Transmission.
	tr := &TRClient{}
	if ConfigSnapshot().ClientType == "" || ConfigSnapshot().ClientType == tr.GetClientType() {
		if tr.Detect() {
			currentClient = tr
			SetCurrentClientType(tr.GetClientType())
			Log("DetectClient", GetLangText("Success-DetectClient"), true, currentClientType)
			return true
		}
	}

	// 最后尝试 BitComet.
	bc := &BCClient{}
	if ConfigSnapshot().ClientType == "" || ConfigSnapshot().ClientType == bc.GetClientType() {
		if bc.Detect() {
			currentClient = bc
			SetCurrentClientType(bc.GetClientType())
			Log("DetectClient", GetLangText("Success-DetectClient"), true, currentClientType)
			return true
		}
	}

	// 如果指定了 ClientType 但探测失败, 则强制创建对应实例.
	if ConfigSnapshot().ClientType != "" {
		SetCurrentClientType(ConfigSnapshot().ClientType)
		switch currentClientType {
		case "qBittorrent":
			currentClient = &QBClient{}
		case "Transmission":
			currentClient = &TRClient{}
		case "BitComet":
			currentClient = &BCClient{}
		}
		return true
	}

	currentClient = nil
	SetCurrentClientType("")
	return false
}

// Login 执行登录操作.
func Login() bool {
	if currentClient != nil {
		ok := currentClient.Login()
		RecordClientResult(ok)
		return ok
	}
	return false
}

// FetchTorrents 获取种子列表.
func FetchTorrents() ([]*Torrent, error) {
	if currentClient != nil {
		items, err := currentClient.FetchTorrents()
		RecordClientResult(err == nil && items != nil)
		return items, err
	}
	return nil, nil
}

// FetchTorrentPeers 获取种子的 Peer 列表.
func FetchTorrentPeers(torrent *Torrent) ([]*Peer, error) {
	if currentClient != nil {
		items, err := currentClient.FetchTorrentPeers(torrent)
		RecordClientResult(err == nil && items != nil)
		return items, err
	}
	return nil, nil
}

// SubmitBlockPeer 提交封禁名单.
func SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	if blockPeerMap == nil {
		return true
	}

	if currentClient != nil {
		if currentClientType == "qBittorrent" && ConfigSnapshot().UseShadowBan {
			return currentClient.SubmitShadowBanPeer(blockPeerMap)
		}
		return currentClient.SubmitBlockPeer(blockPeerMap)
	}

	return false
}

// TestShadowBanAPI 测试静默封禁 API 是否可用.
func TestShadowBanAPI() int {
	// -1: 不支持 (错误), 0: 不支持 (静默), 1: 支持.
	if currentClientType == "qBittorrent" {
		if QB_TestShadowBanAPI() {
			return 1
		}
		return -1
	}

	return 0
}
