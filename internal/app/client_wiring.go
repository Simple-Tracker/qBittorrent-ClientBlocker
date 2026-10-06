package app

import "github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"

// 按端点封禁的 API 默认关闭, 仅在客户端检测选中后启用.
var qB_useNewBanPeersMethod bool

func ClientServices() client.Services {
	return client.Services{
		Snapshot: func() client.Settings {
			cfg := ConfigSnapshot()
			return client.Settings{URL: cfg.ClientURL, Username: cfg.ClientUsername, Password: cfg.ClientPassword,
				BanAllPort: cfg.BanAllPort, NewBanPeersMethod: qB_useNewBanPeersMethod, BlocklistURL: cfg.SyncServerURL + "/ipfilter.dat"}
		},
		SetEndpoint: func(url, username string) {
			UpdateConfig(func(cfg *ConfigStruct) { cfg.ClientURL, cfg.ClientUsername = url, username })
		},
		Fetch: Fetch, Submit: Submit, Now: func() int64 { return currentTimestamp },
		Log: Log, LogError: LogError, Text: GetLangText,
	}
}

// ToClientBans 生成提交数据的快照, 避免向驱动暴露扫描器的映射.
func ToClientBans(peers map[string]BlockPeerInfoStruct) map[string]client.BanTarget {
	result := make(map[string]client.BanTarget, len(peers))
	for ip, peer := range peers {
		ports := make(map[int]bool, len(peer.Port))
		for port, enabled := range peer.Port {
			ports[port] = enabled
		}
		tasks := make(map[string]bool)
		if peer.InfoHash != "" {
			tasks[peer.InfoHash] = true
		}
		for _, hash := range statistics.TasksForIP(ip) {
			if hash != "" {
				tasks[hash] = true
			}
		}
		taskIDs := make([]string, 0, len(tasks))
		for hash := range tasks {
			taskIDs = append(taskIDs, hash)
		}
		result[ip] = client.BanTarget{Ports: ports, TaskIDs: taskIDs}
	}
	return result
}
