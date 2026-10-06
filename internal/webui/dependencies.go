package webui

import (
	"sync"
	"time"
)

type Config struct {
	Enabled            bool
	Username, Password string
}

type Submission struct {
	Pending    bool
	PendingIPs int
	NextRetry  int64
}

// Dependencies 提供不可变的应用快照. 回调必须返回独立的切片,
// 且不得在持有应用锁时回调 WebUI.
type Dependencies struct {
	Config     func() Config
	Status     func() StatusResponse
	BlockStats func() (int, int)
	BlockPeers func() []WebUIBlockPeer
	BlockPeer  func(string) (WebUIBlockPeer, bool)
	LegacyLogs func() []string
	Submission func() Submission
	Now        func() time.Time
}

var dependenciesMutex sync.RWMutex
var dependencies = defaultDependencies()

func defaultDependencies() Dependencies {
	return Dependencies{
		Config:     func() Config { return Config{} },
		Status:     func() StatusResponse { return StatusResponse{} },
		BlockStats: func() (int, int) { return 0, 0 },
		BlockPeers: func() []WebUIBlockPeer { return []WebUIBlockPeer{} },
		BlockPeer:  func(string) (WebUIBlockPeer, bool) { return WebUIBlockPeer{}, false },
		LegacyLogs: func() []string { return []string{} },
		Submission: func() Submission { return Submission{} },
		Now:        time.Now,
	}
}

// Configure 更换数据提供方, 保留 WebUI 自身的历史和健康状态.
func Configure(value Dependencies) {
	defaults := defaultDependencies()
	if value.Config == nil {
		value.Config = defaults.Config
	}
	if value.Status == nil {
		value.Status = defaults.Status
	}
	if value.BlockStats == nil {
		value.BlockStats = defaults.BlockStats
	}
	if value.BlockPeers == nil {
		value.BlockPeers = defaults.BlockPeers
	}
	if value.BlockPeer == nil {
		value.BlockPeer = defaults.BlockPeer
	}
	if value.LegacyLogs == nil {
		value.LegacyLogs = defaults.LegacyLogs
	}
	if value.Submission == nil {
		value.Submission = defaults.Submission
	}
	if value.Now == nil {
		value.Now = defaults.Now
	}
	dependenciesMutex.Lock()
	dependencies = value
	dependenciesMutex.Unlock()
}

func dependenciesSnapshot() Dependencies {
	dependenciesMutex.RLock()
	defer dependenciesMutex.RUnlock()
	return dependencies
}

func now() time.Time { return dependenciesSnapshot().Now() }
