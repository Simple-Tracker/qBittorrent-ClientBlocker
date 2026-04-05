package main

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"time"
)

//go:embed webui.html
var WebUI_Index_HTML []byte

type StatusResponse struct {
	ProgramName    string  `json:"program_name"`
	ProgramVersion string  `json:"program_version"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	ClientType     string  `json:"client_type"`
	ClientURL      string  `json:"client_url"`
	CurrentStats   Stats   `json:"stats"`
	BTNStatus      string  `json:"btn_status"`
	Runtime        Runtime `json:"runtime"`
}

type Stats struct {
	TotalBlockedIPs     int   `json:"total_blocked_ips"`
	TotalBlockedPorts   int   `json:"total_blocked_ports"`
	LastUpdateTimestamp int64 `json:"last_update_timestamp"`
}

type Runtime struct {
	GoVersion    string `json:"go_version"`
	NumGoroutine int    `json:"num_goroutine"`
	AllocMB      uint64 `json:"alloc_mb"`
}

type WebUIBlockPeer struct {
	IP        string   `json:"ip"`
	Timestamp int64    `json:"timestamp"`
	Module    string   `json:"module"`
	Reason    string   `json:"reason"`
	Ports     []string `json:"ports"`
}

var startTimestamp = time.Now().Unix()

func WebUI_IsPath(path string) bool {
	return path == "/" || path == "/api/status" || path == "/api/peers" || path == "/api/logs"
}

func WebUI_CheckBasicAuth(w http.ResponseWriter, r *http.Request) bool {
	if config.WebUIUsername == "" {
		return true
	}

	username, password, ok := r.BasicAuth()
	if ok &&
		subtle.ConstantTimeCompare([]byte(username), []byte(config.WebUIUsername)) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(config.WebUIPassword)) == 1 {
		return true
	}

	w.Header().Set("WWW-Authenticate", `Basic realm="qBittorrent-ClientBlocker WebUI", charset="UTF-8"`)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("401: Unauthorized."))
	return false
}

func GetWebUIBlockStats() (int, int) {
	blockPeerMapMutex.RLock()
	defer blockPeerMapMutex.RUnlock()

	totalBlockedIPs := len(blockPeerMap)
	totalBlockedPorts := 0
	for _, peerInfo := range blockPeerMap {
		totalBlockedPorts += len(peerInfo.Port)
	}

	return totalBlockedIPs, totalBlockedPorts
}

func GetWebUIBlockPeers() []WebUIBlockPeer {
	blockPeerMapMutex.RLock()
	defer blockPeerMapMutex.RUnlock()

	peers := make([]WebUIBlockPeer, 0, len(blockPeerMap))
	for peerIP, peerInfo := range blockPeerMap {
		ports := make([]int, 0, len(peerInfo.Port))
		for port := range peerInfo.Port {
			ports = append(ports, port)
		}
		sort.Ints(ports)

		portLabels := make([]string, 0, len(ports))
		for _, port := range ports {
			if port == -1 {
				portLabels = append(portLabels, "ALL")
				continue
			}
			portLabels = append(portLabels, strconv.Itoa(port))
		}

		peers = append(peers, WebUIBlockPeer{
			IP:        peerIP,
			Timestamp: peerInfo.Timestamp,
			Module:    peerInfo.Module,
			Reason:    peerInfo.Reason,
			Ports:     portLabels,
		})
	}

	sort.Slice(peers, func(i, j int) bool {
		if peers[i].Timestamp == peers[j].Timestamp {
			return peers[i].IP < peers[j].IP
		}
		return peers[i].Timestamp > peers[j].Timestamp
	})

	return peers
}

func WebUI_GetStatus(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	btnStatus := "Not Loaded"
	if btnConfig != nil {
		if ability, exists := btnConfig.Ability["reconfigure"]; exists {
			btnStatus = "Active (" + ability.Version + ")"
		} else {
			btnStatus = "Active"
		}
	}

	totalBlockedIPs, totalBlockedPorts := GetWebUIBlockStats()
	stats := Stats{
		TotalBlockedIPs:     totalBlockedIPs,
		TotalBlockedPorts:   totalBlockedPorts,
		LastUpdateTimestamp: atomic.LoadInt64(&currentTimestamp),
	}

	resp := StatusResponse{
		ProgramName:    programName,
		ProgramVersion: programVersion,
		UptimeSeconds:  time.Now().Unix() - startTimestamp,
		ClientType:     currentClientType,
		ClientURL:      config.ClientURL,
		CurrentStats:   stats,
		BTNStatus:      btnStatus,
		Runtime: Runtime{
			GoVersion:    runtime.Version(),
			NumGoroutine: runtime.NumGoroutine(),
			AllocMB:      m.Alloc / 1024 / 1024,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func WebUI_GetPeers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetWebUIBlockPeers())
}

func WebUI_GetLogs(w http.ResponseWriter, r *http.Request) {
	logBufferMutex.Lock()
	defer logBufferMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logBuffer)
}

func WebUI_Index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// 优先检查当前目录是否有外部 webui.html.
	const externalFile = "webui.html"
	if _, err := os.Stat(externalFile); err == nil {
		if content, err := os.ReadFile(externalFile); err == nil {
			w.Write(content)
			return
		}
	}

	w.Write(WebUI_Index_HTML)
}
