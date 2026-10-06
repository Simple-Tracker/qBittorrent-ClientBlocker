package app

import "github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/stats"

var statistics = stats.NewStore(stats.Options{
	Settings: func() stats.Settings {
		cfg := ConfigSnapshot()
		return stats.Settings{
			Interval:                      cfg.Interval,
			HistoryRetention:              cfg.HistoryRetention,
			HistoryMaxEntries:             cfg.HistoryMaxEntries,
			TorrentMapCleanInterval:       cfg.TorrentMapCleanInterval,
			SyncServerURL:                 cfg.SyncServerURL,
			BTNSubmitPeers:                cfg.BTNSubmitPeers,
			BTNSubmitHistories:            cfg.BTNSubmitHistories,
			IPUploadedCheck:               cfg.IPUploadedCheck,
			IPUpCheckInterval:             cfg.IPUpCheckInterval,
			IPUpCheckIncrementMB:          cfg.IPUpCheckIncrementMB,
			MaxIPPortCount:                cfg.MaxIPPortCount,
			IPUpCheckPerTorrentRatio:      cfg.IPUpCheckPerTorrentRatio,
			BanByProgressUploaded:         cfg.BanByProgressUploaded,
			BanByPUStartMB:                cfg.BanByPUStartMB,
			BanByPUStartPercent:           cfg.BanByPUStartPercent,
			BanByPUAntiErrorRatio:         cfg.BanByPUAntiErrorRatio,
			BanByRelativeProgressUploaded: cfg.BanByRelativeProgressUploaded,
			BanByRelativePUStartMB:        cfg.BanByRelativePUStartMB,
			BanByRelativePUStartPercent:   cfg.BanByRelativePUStartPercent,
			BanByRelativePUAntiErrorRatio: cfg.BanByRelativePUAntiErrorRatio,
		}
	},
	Now:       func() int64 { return currentTimestamp },
	IsBlocked: func(ip string, port int) bool { return IsBlockedPeer(ip, port, false) },
	Block: func(event stats.BlockEvent) {
		AddBlockPeer(event.Module, event.Reason, event.IP, event.Port, event.InfoHash, event.ID, event.Client, event.Downloaded, event.Uploaded)
		if event.Counters != nil {
			SeedBlockedPeerCounters(event.IP, event.Counters)
		}
		AddBlockCIDR(event.IP, event.Net)
	},
	CIDR: ParseIPCIDRByConfig,
	Log:  Log,
})
