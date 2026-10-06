package app

import "testing"

func TestBTNDoesNotRecreateExpiredConnections(t *testing.T) {
	reports := installBTNProtocolTest(t)
	UpdateConfig(func(c *ConfigStruct) { c.HistoryRetention, c.Interval = 120, 1 })
	processCIDRTestPeer("203.0.113.10", 6881, .5, 1<<20)
	currentTimestamp = 250
	statistics.CheckAllTorrent()
	// 检测可能在 CleanHistory 删除上层记录前清理端口.
	BTN_SubmitPeers(statistics.State().TorrentMap, currentTimestamp)
	if got := <-reports; len(got.peers) != 0 {
		t.Fatalf("expired endpoint reappeared in peer snapshot: %+v", got.peers)
	}
	BTN_SubmitHistories(statistics.State().TorrentMap, statistics.State().LastTorrentMap, currentTimestamp)
	if got := <-reports; len(got.histories) != 0 {
		t.Fatalf("expired endpoint reappeared in history: %+v", got.histories)
	}
}
