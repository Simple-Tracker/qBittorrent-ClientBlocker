package webui

import (
	"testing"
	"time"
)

func TestWebUIHealthOwnsPublishedState(t *testing.T) {
	old := dependenciesSnapshot()
	t.Cleanup(func() { Configure(old); ResetWebUIHealth() })
	clock := int64(100)
	submission := Submission{Pending: true, PendingIPs: 2, NextRetry: 110}
	Configure(Dependencies{Now: func() time.Time { return time.Unix(clock, 0) }, Submission: func() Submission { return submission }})
	ResetWebUIHealth()
	StartWebUIScan()
	RecordClientResult(false)
	PublishSubmission("retrying")
	FinishWebUIScan(false)
	h := WebUIHealthSnapshot()
	if h.ScanState != "failed" || h.ClientState != "failed" || h.LastClientFailure != 100 || h.LastSubmissionFailure != 100 || !h.Pending || h.PendingIPs != 2 || h.NextRetry != 110 {
		t.Fatalf("failed: %+v", h)
	}
	clock = 200
	submission = Submission{}
	StartWebUIScan()
	RecordClientResult(true)
	PublishSubmission("succeeded")
	FinishWebUIScan(true)
	h = WebUIHealthSnapshot()
	if h.ScanState != "succeeded" || h.LastScanSuccess != 200 || h.LastClientSuccess != 200 || h.LastSubmissionSuccess != 200 || h.Pending {
		t.Fatalf("recovered: %+v", h)
	}
	StartWebUIScan()
	RecordClientResult(false)
	RecordClientResult(true)
	FinishWebUIScan(true)
	if WebUIHealthSnapshot().ScanState != "failed" {
		t.Fatal("partial scan reported success")
	}
}
