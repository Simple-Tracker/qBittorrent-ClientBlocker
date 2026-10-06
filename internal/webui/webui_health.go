package webui

import (
	"encoding/json"
	"net/http"
	"sync"
)

// 发布的快照使 HTTP 读取方无需访问扫描器的可变状态.
type WebUIHealth struct {
	ClientState           string `json:"client_state"`
	LastClientSuccess     int64  `json:"last_client_success"`
	LastClientFailure     int64  `json:"last_client_failure"`
	ScanState             string `json:"scan_state"`
	LastScanStarted       int64  `json:"last_scan_started"`
	LastScanSuccess       int64  `json:"last_scan_success"`
	SubmissionState       string `json:"submission_state"`
	LastSubmissionSuccess int64  `json:"last_submission_success"`
	LastSubmissionFailure int64  `json:"last_submission_failure"`
	Pending               bool   `json:"pending"`
	PendingIPs            int    `json:"pending_ips"`
	NextRetry             int64  `json:"next_retry"`
}

var webUIHealthMutex sync.Mutex
var webUIHealth = WebUIHealth{ClientState: "unknown", ScanState: "idle", SubmissionState: "idle"}
var webUIScanFailed bool

func ResetWebUIHealth() {
	webUIHealthMutex.Lock()
	defer webUIHealthMutex.Unlock()
	webUIHealth = WebUIHealth{ClientState: "unknown", ScanState: "idle", SubmissionState: "idle"}
	webUIScanFailed = false
}
func RecordClientResult(ok bool) {
	webUIHealthMutex.Lock()
	defer webUIHealthMutex.Unlock()
	if ok {
		webUIHealth.ClientState = "reachable"
		webUIHealth.LastClientSuccess = now().Unix()
	} else {
		webUIHealth.ClientState = "failed"
		webUIHealth.LastClientFailure = now().Unix()
		if webUIHealth.ScanState == "running" {
			webUIScanFailed = true
		}
	}
}
func StartWebUIScan() {
	webUIHealthMutex.Lock()
	defer webUIHealthMutex.Unlock()
	webUIHealth.ScanState = "running"
	webUIHealth.LastScanStarted = now().Unix()
	webUIScanFailed = false
}
func FinishWebUIScan(completed bool) {
	webUIHealthMutex.Lock()
	defer webUIHealthMutex.Unlock()
	if completed && !webUIScanFailed {
		webUIHealth.ScanState = "succeeded"
		webUIHealth.LastScanSuccess = now().Unix()
	} else {
		webUIHealth.ScanState = "failed"
	}
}
func PublishSubmission(state string) {
	webUIHealthMutex.Lock()
	defer webUIHealthMutex.Unlock()
	submission := dependenciesSnapshot().Submission()
	webUIHealth.Pending = submission.Pending
	webUIHealth.PendingIPs = 0
	if submission.Pending {
		webUIHealth.PendingIPs = submission.PendingIPs
	}
	webUIHealth.NextRetry = submission.NextRetry
	if state != "" {
		webUIHealth.SubmissionState = state
	}
	if state == "succeeded" {
		webUIHealth.LastSubmissionSuccess = now().Unix()
	}
	if state == "retrying" {
		webUIHealth.LastSubmissionFailure = now().Unix()
	}
}
func WebUIHealthSnapshot() WebUIHealth {
	webUIHealthMutex.Lock()
	defer webUIHealthMutex.Unlock()
	return webUIHealth
}
func WebUI_GetHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(WebUIHealthSnapshot())
}
