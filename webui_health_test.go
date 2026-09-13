package main

import "testing"

func TestWebUIHealthTracksRetryAndRecovery(t *testing.T) {
	old := WebUIHealthSnapshot()
	t.Cleanup(func() { webUIHealthMutex.Lock(); webUIHealth = old; webUIHealthMutex.Unlock() })
	client := InstallRetryTestClient(t)
	ResetWebUIHealth()
	client.failFetch = true
	Task()
	h := WebUIHealthSnapshot()
	if h.ClientState != "failed" || h.ScanState != "failed" || !h.Pending || h.PendingIPs != 2 || h.SubmissionState != "retrying" || h.NextRetry <= 100 || h.LastScanSuccess != 0 {
		t.Fatalf("failed: %+v", h)
	}
	client.failFetch = false
	client.failSubmit = false
	currentTimestamp = 200
	Task()
	h = WebUIHealthSnapshot()
	if h.ClientState != "reachable" || h.ScanState != "succeeded" || h.Pending || h.SubmissionState != "succeeded" || h.NextRetry != 0 || h.LastScanSuccess == 0 || h.LastSubmissionSuccess == 0 {
		t.Fatalf("recovered: %+v", h)
	}
	StartWebUIScan()
	RecordClientResult(false)
	RecordClientResult(true)
	FinishWebUIScan(true)
	if WebUIHealthSnapshot().ScanState != "failed" {
		t.Fatal("partial scan reported success")
	}
	ResetWebUIHealth()
	if WebUIHealthSnapshot().LastScanSuccess != 0 {
		t.Fatal("reload retained old success")
	}
}
