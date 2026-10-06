// stats 包负责 Peer 流量采样, 历史记录和统计检测.
package stats

import (
	"net"
	"sync"
	"time"
)

// Settings 仅包含统计采样和规则所需的配置.
type Settings struct {
	Interval, HistoryRetention, HistoryMaxEntries, TorrentMapCleanInterval uint32
	SyncServerURL                                                          string
	BTNSubmitPeers, BTNSubmitHistories                                     bool
	IPUploadedCheck                                                        bool
	IPUpCheckInterval, IPUpCheckIncrementMB, MaxIPPortCount                uint32
	IPUpCheckPerTorrentRatio                                               float64
	BanByProgressUploaded                                                  bool
	BanByPUStartMB                                                         uint32
	BanByPUStartPercent, BanByPUAntiErrorRatio                             float64
	BanByRelativeProgressUploaded                                          bool
	BanByRelativePUStartMB                                                 uint32
	BanByRelativePUStartPercent, BanByRelativePUAntiErrorRatio             float64
}

// BlockEvent 描述统计检测发现的违规及其计数基线.
// 应用层负责执行封禁并记录其流量.
type BlockEvent struct {
	Module, Reason, IP, InfoHash, ID, Client string
	Port                                     int
	Downloaded, Uploaded                     int64
	Net                                      *net.IPNet
	Counters                                 map[string]map[int]PeerTrafficCounter
}

// Options 注入应用策略, 使 stats 无需依赖应用层.
// 回调同步执行, 不得再次调用同一个 Store.
type Options struct {
	Settings  func() Settings
	Now       func() int64
	IsBlocked func(ip string, port int) bool
	Block     func(BlockEvent)
	CIDR      func(ip string) *net.IPNet
	Log       func(module, format string, print bool, args ...any)
}

// State 提供状态访问以构造确定性的测试数据. 生产代码读取时应使用快照;
// 修改实时 map 或替换状态前, 必须保证 Store 没有正在执行的操作.
type State struct {
	IPMap, LastIPMap                                     map[string]IPInfoStruct
	TorrentMap, LastTorrentMap                           map[string]TorrentInfoStruct
	IPMutex, LastIPMutex, TorrentMutex, LastTorrentMutex sync.RWMutex
	LastIPClean, LastTorrentClean, LastHistoryClean      int64
}

// Store 管理所有采样状态. 零值不可直接使用; 请通过 NewStore 创建.
type Store struct {
	state        *State
	options      Options
	historyMutex sync.Mutex
}

func NewStore(options Options) *Store {
	s := &Store{options: options}
	s.ReplaceState(nil)
	return s
}

// State 不加锁地返回实时状态, 仅用于测试初始化.
func (s *Store) State() *State { return s.state }

// ReplaceState 设置测试状态并保留注入的策略, 仅可在 Store 没有运行中的操作时调用.
// 传入 nil 时创建空的历史记录.
func (s *Store) ReplaceState(state *State) {
	if state == nil {
		state = &State{}
	}
	if state.IPMap == nil {
		state.IPMap = make(map[string]IPInfoStruct)
	}
	if state.LastIPMap == nil {
		state.LastIPMap = make(map[string]IPInfoStruct)
	}
	if state.TorrentMap == nil {
		state.TorrentMap = make(map[string]TorrentInfoStruct)
	}
	if state.LastTorrentMap == nil {
		state.LastTorrentMap = make(map[string]TorrentInfoStruct)
	}
	s.state = state
}

func (s *Store) settings() *Settings {
	if s.options.Settings == nil {
		return &Settings{}
	}
	settings := s.options.Settings()
	return &settings
}
func (s *Store) now() int64 {
	if s.options.Now != nil {
		return s.options.Now()
	}
	return time.Now().Unix()
}
func (s *Store) network(ip string) *net.IPNet {
	if s.options.CIDR == nil {
		return nil
	}
	return s.options.CIDR(ip)
}
func (s *Store) isBlocked(ip string, port int) bool {
	return s.options.IsBlocked != nil && s.options.IsBlocked(ip, port)
}
func (s *Store) block(event BlockEvent) {
	if s.options.Block != nil {
		s.options.Block(event)
	}
}
func (s *Store) log(module, format string, print bool, args ...any) {
	if s.options.Log != nil {
		s.options.Log(module, format, print, args...)
	}
}

func (s *Store) blockTorrentPeer(reason, ip string, blockPort int, hash string, connectionPort int, peer PeerInfoStruct, hasConnections bool) {
	event := BlockEvent{Module: "CheckAllTorrent", Reason: reason, IP: ip, Port: blockPort, InfoHash: hash, ID: peer.ID, Client: peer.Client, Downloaded: peer.Downloaded, Uploaded: peer.Uploaded, Net: peer.Net}
	if hasConnections {
		event.Counters = map[string]map[int]PeerTrafficCounter{hash: {connectionPort: peer.Counters}}
	}
	s.block(event)
}

// HasPeer 判断任一采样路径是否保留了该连接.
func (s *Store) HasPeer(ip, hash string, port int) bool {
	s.state.IPMutex.RLock()
	_, exists := s.state.IPMap[ip].TorrentPeers[hash][port]
	s.state.IPMutex.RUnlock()
	if exists {
		return true
	}
	s.state.TorrentMutex.RLock()
	_, exists = s.state.TorrentMap[hash].Peers[ip].Connections[port]
	s.state.TorrentMutex.RUnlock()
	return exists
}

// TasksForIP 返回 IP 采样保留的种子标识.
func (s *Store) TasksForIP(ip string) []string {
	s.state.IPMutex.RLock()
	defer s.state.IPMutex.RUnlock()
	result := make([]string, 0, len(s.state.IPMap[ip].TorrentLastSeen))
	for hash := range s.state.IPMap[ip].TorrentLastSeen {
		result = append(result, hash)
	}
	return result
}
