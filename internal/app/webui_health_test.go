package app

import (
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/webui"
)

func TestWebUIHealthTracksRetryAndRecovery(t *testing.T) {
	webui.ResetWebUIHealth()
	t.Cleanup(webui.ResetWebUIHealth)
	client := InstallRetryTestClient(t)
	webui.ResetWebUIHealth()
	client.failFetch = true
	Task()
	h := webui.WebUIHealthSnapshot()
	if h.ClientState != "failed" || h.ScanState != "failed" || !h.Pending || h.PendingIPs != 2 || h.SubmissionState != "retrying" || h.NextRetry <= 100 || h.LastScanSuccess != 0 {
		t.Fatalf("failed: %+v", h)
	}
	client.failFetch = false
	client.failSubmit = false
	currentTimestamp = 200
	Task()
	h = webui.WebUIHealthSnapshot()
	if h.ClientState != "reachable" || h.ScanState != "succeeded" || h.Pending || h.SubmissionState != "succeeded" || h.NextRetry != 0 || h.LastScanSuccess == 0 || h.LastSubmissionSuccess == 0 {
		t.Fatalf("recovered: %+v", h)
	}
	webui.StartWebUIScan()
	webui.RecordClientResult(false)
	webui.RecordClientResult(true)
	webui.FinishWebUIScan(true)
	if webui.WebUIHealthSnapshot().ScanState != "failed" {
		t.Fatal("partial scan reported success")
	}
	webui.ResetWebUIHealth()
	if webui.WebUIHealthSnapshot().LastScanSuccess != 0 {
		t.Fatal("reload retained old success")
	}
}
