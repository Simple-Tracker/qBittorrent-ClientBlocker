package app

import (
	"errors"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

type retryTestClient struct {
	coverageClient
	failFetch   bool
	failSubmit  bool
	submissions []int
}

func (c *retryTestClient) FetchTorrents() ([]*Torrent, error) {
	if c.failFetch {
		return nil, errors.New("client unavailable")
	}
	return []*Torrent{}, nil
}

func (c *retryTestClient) SubmitBlockPeer(peers map[string]client.BanTarget) bool {
	c.submissions = append(c.submissions, len(peers))
	return !c.failSubmit
}

func InstallRetryTestClient(t *testing.T) *retryTestClient {
	t.Helper()
	InstallScreenshotScalePeers(t, map[string]BlockPeerInfoStruct{
		"192.0.2.1":    {Timestamp: 1, Port: map[int]bool{6881: true}},
		"198.51.100.1": {Timestamp: 100, Port: map[int]bool{6881: true}},
	})
	oldClient, oldSubmission := currentClient, blockPeerSubmission
	oldRules, oldSyncConfig := syncServer_CompiledRules, syncServer_syncConfig
	t.Cleanup(func() {
		currentClient, blockPeerSubmission = oldClient, oldSubmission
		syncServer_CompiledRules, syncServer_syncConfig = oldRules, oldSyncConfig
	})
	blockPeerSubmission.Pending = false
	blockPeerSubmission.Next, blockPeerSubmission.Delay = 0, 0
	config.ClientURL = "http://client.test"
	config.UseShadowBan = false
	config.BanTime = 10
	config.GenIPDat = 0
	config.MaxIPPortCount = 0
	config.IPUploadedCheck = false
	config.BanByRelativeProgressUploaded = false
	config.BTNSubmitHistories = false
	config.SyncServerURL, config.BTNConfigureURL = "", ""
	currentTimestamp = 100
	client := &retryTestClient{failSubmit: true}
	currentClient = client
	return client
}

func TestTaskRetriesFailedBanWithoutNewPeers(t *testing.T) {
	client := InstallRetryTestClient(t)
	Task() // 过期删除触发提交, 失败后待重试.
	if len(client.submissions) != 1 || client.submissions[0] != 1 || !blockPeerSubmission.Pending {
		t.Fatalf("initial submission=%v pending=%t", client.submissions, blockPeerSubmission.Pending)
	}
	currentTimestamp = 101
	Task() // 无新增或删除, 仍须重试.
	if len(client.submissions) != 2 || blockPeerSubmission.Next != 103 {
		t.Fatalf("retry submissions=%v next=%d", client.submissions, blockPeerSubmission.Next)
	}
	currentTimestamp = 102
	Task()
	if len(client.submissions) != 2 {
		t.Fatal("retried before backoff expired")
	}
	config.BanTime = 1
	currentTimestamp = 103
	client.failSubmit = false
	Task() // 等待期间剩余 IP 也过期, 应提交最新的空名单.
	if len(client.submissions) != 3 || client.submissions[2] != 0 || blockPeerSubmission.Pending {
		t.Fatalf("latest submission=%v pending=%t", client.submissions, blockPeerSubmission.Pending)
	}
	currentTimestamp = 104
	Task()
	if len(client.submissions) != 3 {
		t.Fatal("successful submission was needlessly repeated")
	}
}

func TestTaskResubmitsAfterClientFetchFailure(t *testing.T) {
	client := InstallRetryTestClient(t)
	client.failSubmit = false
	Task()
	client.failSubmit, client.failFetch = true, true
	currentTimestamp++
	Task()
	if !blockPeerSubmission.Pending || len(client.submissions) != 2 {
		t.Fatal("disconnect did not preserve pending ban state")
	}
	client.failSubmit, client.failFetch = false, false
	currentTimestamp++
	Task()
	if blockPeerSubmission.Pending || len(client.submissions) != 3 || client.submissions[2] != 1 {
		t.Fatalf("reconnect submissions=%v pending=%t", client.submissions, blockPeerSubmission.Pending)
	}
}

func TestBanSubmissionBackoffIsBounded(t *testing.T) {
	InstallRetryTestClient(t)
	blockPeerSubmission.Pending = true
	for _, delay := range []int64{1, 2, 4, 8, 16, 32, 60, 60} {
		RetryBlockPeerSubmission()
		if blockPeerSubmission.Delay != delay || blockPeerSubmission.Next != currentTimestamp+delay {
			t.Fatalf("backoff=%d next=%d, want %d seconds", blockPeerSubmission.Delay, blockPeerSubmission.Next, delay)
		}
		currentTimestamp = blockPeerSubmission.Next
	}
}
