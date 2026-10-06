package app

import (
	"sync"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/bitcomet"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/qbittorrent"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/transmission"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/webui"
)

// currentClient 是当前正在使用的客户端实例.
var currentClient Client

// currentClientType 是当前客户端的类型名称, 如 "qBittorrent".
var currentClientType = ""
var clientStateMutex sync.RWMutex

// CurrentClientSnapshot 返回相互一致的客户端实例和类型.
// 必须等本函数返回并释放状态锁后, 才能调用驱动.
func CurrentClientSnapshot() (Client, string) {
	clientStateMutex.RLock()
	defer clientStateMutex.RUnlock()
	return currentClient, currentClientType
}

func SetCurrentClient(instance Client, name string) {
	clientStateMutex.Lock()
	currentClient, currentClientType = instance, name
	clientStateMutex.Unlock()
}

func CurrentClientTypeSnapshot() string {
	_, name := CurrentClientSnapshot()
	return name
}

func SetCurrentClientType(name string) {
	clientStateMutex.Lock()
	currentClientType = name
	clientStateMutex.Unlock()
}

// IsBanPort 返回当前客户端是否支持按端口封禁.
func IsBanPort() bool {
	if CurrentClientTypeSnapshot() == "qBittorrent" && qB_useNewBanPeersMethod {
		return true
	}

	return false
}

// IsSupportClient 返回当前是否已检测到支持的客户端.
func IsSupportClient() bool {
	instance, _ := CurrentClientSnapshot()
	return instance != nil
}

// InitClient 初始化客户端特定功能.
func InitClient() {
	if CurrentClientTypeSnapshot() == "Transmission" {
		GoWithCrashLog("Transmission.StartServer", StartServer)
	}
}

// SetURLFromClient 尝试从本地配置文件读取并设置客户端 API 地址.
func SetURLFromClient() {
	if ConfigSnapshot().ClientURL == "" {
		qb := qbittorrent.New(ClientServices())
		if !qb.SetURL() {
			tr := transmission.New(ClientServices())
			tr.SetURL()
		}
	}
}

// DetectClient 自动探测当前使用的下载软件类型.
func DetectClient() bool {
	// 先尝试 qBittorrent.
	qb := qbittorrent.New(ClientServices())
	if ConfigSnapshot().ClientType == "" || ConfigSnapshot().ClientType == qb.GetClientType() {
		if qb.Detect() {
			SetCurrentClient(qb, qb.GetClientType())
			Log("DetectClient", GetLangText("Success-DetectClient"), true, qb.GetClientType())
			return true
		}
	}

	// 再尝试 Transmission.
	tr := transmission.New(ClientServices())
	if ConfigSnapshot().ClientType == "" || ConfigSnapshot().ClientType == tr.GetClientType() {
		if tr.Detect() {
			SetCurrentClient(tr, tr.GetClientType())
			Log("DetectClient", GetLangText("Success-DetectClient"), true, tr.GetClientType())
			return true
		}
	}

	// 最后尝试 BitComet.
	bc := bitcomet.New(ClientServices())
	if ConfigSnapshot().ClientType == "" || ConfigSnapshot().ClientType == bc.GetClientType() {
		if bc.Detect() {
			SetCurrentClient(bc, bc.GetClientType())
			Log("DetectClient", GetLangText("Success-DetectClient"), true, bc.GetClientType())
			return true
		}
	}

	// 如果指定了 ClientType 但探测失败, 则强制创建对应实例.
	if name := ConfigSnapshot().ClientType; name != "" {
		instance, _ := CurrentClientSnapshot()
		switch name {
		case "qBittorrent":
			instance = qbittorrent.New(ClientServices())
		case "Transmission":
			instance = transmission.New(ClientServices())
		case "BitComet":
			instance = bitcomet.New(ClientServices())
		}
		SetCurrentClient(instance, name)
		return true
	}

	SetCurrentClient(nil, "")
	return false
}

// Login 执行登录操作.
func Login() bool {
	if instance, _ := CurrentClientSnapshot(); instance != nil {
		ok := instance.Login()
		webui.RecordClientResult(ok)
		return ok
	}
	return false
}

// FetchTorrents 获取种子列表.
func FetchTorrents() ([]*Torrent, error) {
	if instance, _ := CurrentClientSnapshot(); instance != nil {
		items, err := instance.FetchTorrents()
		webui.RecordClientResult(err == nil && items != nil)
		return items, err
	}
	return nil, nil
}

// FetchTorrentPeers 获取种子的 Peer 列表.
func FetchTorrentPeers(torrent *Torrent) ([]*Peer, error) {
	if instance, _ := CurrentClientSnapshot(); instance != nil {
		items, err := instance.FetchTorrentPeers(torrent)
		webui.RecordClientResult(err == nil && items != nil)
		return items, err
	}
	return nil, nil
}

// SubmitBlockPeer 提交封禁名单.
func SubmitBlockPeer(blockPeerMap map[string]BlockPeerInfoStruct) bool {
	if blockPeerMap == nil {
		return true
	}

	if instance, name := CurrentClientSnapshot(); instance != nil {
		if name == "qBittorrent" && ConfigSnapshot().UseShadowBan {
			return instance.SubmitShadowBanPeer(ToClientBans(blockPeerMap))
		}
		return instance.SubmitBlockPeer(ToClientBans(blockPeerMap))
	}

	return false
}

// TestShadowBanAPI 测试静默封禁 API 是否可用.
func TestShadowBanAPI() int {
	// -1: 不支持 (错误), 0: 不支持 (静默), 1: 支持.
	if instance, name := CurrentClientSnapshot(); name == "qBittorrent" {
		if capable, ok := instance.(client.ShadowBanTester); ok && capable.TestShadowBanAPI() {
			return 1
		}
		return -1
	}

	return 0
}
