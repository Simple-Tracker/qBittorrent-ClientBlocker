package app

import (
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/stats"
)

func InstallHistoryTest(t *testing.T) {
	t.Helper()
	oldCfg := ConfigSnapshot()
	oldIP, oldLastIP, oldTorrent, oldLastTorrent := statistics.State().IPMap, statistics.State().LastIPMap, statistics.State().TorrentMap, statistics.State().LastTorrentMap
	oldNow, oldClean := currentTimestamp, statistics.State().LastHistoryClean
	t.Cleanup(func() {
		ReplaceConfig(oldCfg)
		statistics.State().IPMap, statistics.State().LastIPMap, statistics.State().TorrentMap, statistics.State().LastTorrentMap = oldIP, oldLastIP, oldTorrent, oldLastTorrent
		currentTimestamp, statistics.State().LastHistoryClean = oldNow, oldClean
	})
	cfg := *oldCfg
	cfg.HistoryRetention, cfg.HistoryMaxEntries = 120, 200
	cfg.Interval, cfg.IPUpCheckInterval, cfg.TorrentMapCleanInterval = 1, 1, 1
	cfg.MaxIPPortCount, cfg.IPUpCheckIncrementMB = 10, 1
	cfg.IPUploadedCheck, cfg.BanByRelativeProgressUploaded = true, true
	ReplaceConfig(&cfg)
	statistics.State().IPMap, statistics.State().LastIPMap = make(map[string]stats.IPInfoStruct), make(map[string]stats.IPInfoStruct)
	statistics.State().TorrentMap, statistics.State().LastTorrentMap = make(map[string]stats.TorrentInfoStruct), make(map[string]stats.TorrentInfoStruct)
	currentTimestamp, statistics.State().LastHistoryClean = 100, 0
}
