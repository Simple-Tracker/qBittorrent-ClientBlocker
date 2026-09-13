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
	"sync"
	"sync/atomic"
	"time"
)

//go:embed webui.html
var WebUI_Index_HTML []byte

type StatusResponse struct {
	ProgramName      string   `json:"program_name"`
	ProgramVersion   string   `json:"program_version"`
	UptimeSeconds    int64    `json:"uptime_seconds"`
	ClientType       string   `json:"client_type"`
	ClientURL        string   `json:"client_url"`
	LoadedExtensions []string `json:"loaded_extensions"`
	CurrentStats     Stats    `json:"stats"`
	Runtime          Runtime  `json:"runtime"`
}

type Stats struct {
	TotalBlockedIPs     int   `json:"total_blocked_ips"`
	TotalBlockedPorts   int   `json:"total_blocked_ports"`
	LastUpdateTimestamp int64 `json:"last_update_timestamp"`
}

type Runtime struct {
	GoVersion    string `json:"go_version"`
	NumGoroutine int    `json:"num_goroutine"`
}

type WebUIBlockPeer struct {
	IP         string   `json:"ip"`
	Timestamp  int64    `json:"timestamp"`
	Module     string   `json:"module"`
	Reason     string   `json:"reason"`
	Ports      []string `json:"ports"`
	ID         string   `json:"id"`
	Client     string   `json:"client"`
	Downloaded int64    `json:"downloaded"`
	Uploaded   int64    `json:"uploaded"`
}

type WebUIBlockPeerSyncResponse struct {
	Epoch     string           `json:"epoch"`
	Reset     bool             `json:"reset"`
	Cursor    uint64           `json:"cursor"`
	Peers     []WebUIBlockPeer `json:"peers"`
	RemovedIP []string         `json:"removed_ips"`
}

type webUIBlockPeerEvent struct {
	Cursor    uint64
	Peer      *WebUIBlockPeer
	RemovedIP string
}

const webUIMaxPeerEvents = 4096

var webUIPeerSyncMutex sync.Mutex
var webUIPeerSyncEpoch = strconv.FormatInt(time.Now().UnixNano(), 36)
var webUIPeerSyncCursor uint64
var webUIPeerSyncEvents []webUIBlockPeerEvent
var webUIPeerSyncEventStart int

func ResetWebUIPeerSync() {
	webUIPeerSyncMutex.Lock()
	defer webUIPeerSyncMutex.Unlock()
	webUIPeerSyncEpoch = strconv.FormatInt(time.Now().UnixNano(), 36)
	webUIPeerSyncCursor, webUIPeerSyncEventStart = 0, 0
	webUIPeerSyncEvents = nil
}

func WebUI_IsPath(path string) bool {
	return path == "/" || path == "/api/status" || path == "/api/peers" || path == "/api/logs"
}

func WebUI_CheckBasicAuth(w http.ResponseWriter, r *http.Request) bool {
	if ConfigSnapshot().WebUIUsername == "" {
		return true
	}

	username, password, ok := r.BasicAuth()
	if ok &&
		subtle.ConstantTimeCompare([]byte(username), []byte(ConfigSnapshot().WebUIUsername)) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(ConfigSnapshot().WebUIPassword)) == 1 {
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
			IP:         peerIP,
			Timestamp:  peerInfo.Timestamp,
			Module:     peerInfo.Module,
			Reason:     peerInfo.Reason,
			Ports:      portLabels,
			ID:         peerInfo.ID,
			Client:     peerInfo.Client,
			Downloaded: peerInfo.Downloaded,
			Uploaded:   peerInfo.Uploaded,
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

func GetWebUIBlockPeer(peerIP string) (WebUIBlockPeer, bool) {
	blockPeerMapMutex.RLock()
	defer blockPeerMapMutex.RUnlock()
	peerInfo, exists := blockPeerMap[peerIP]
	if !exists {
		return WebUIBlockPeer{}, false
	}

	ports := make([]int, 0, len(peerInfo.Port))
	for port := range peerInfo.Port {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	portLabels := make([]string, 0, len(ports))
	for _, port := range ports {
		if port == -1 {
			portLabels = append(portLabels, "ALL")
		} else {
			portLabels = append(portLabels, strconv.Itoa(port))
		}
	}

	return WebUIBlockPeer{
		IP:         peerIP,
		Timestamp:  peerInfo.Timestamp,
		Module:     peerInfo.Module,
		Reason:     peerInfo.Reason,
		Ports:      portLabels,
		ID:         peerInfo.ID,
		Client:     peerInfo.Client,
		Downloaded: peerInfo.Downloaded,
		Uploaded:   peerInfo.Uploaded,
	}, true
}

func AppendWebUIBlockPeerEvent(event webUIBlockPeerEvent) {
	webUIPeerSyncMutex.Lock()
	webUIPeerSyncCursor++
	event.Cursor = webUIPeerSyncCursor
	if len(webUIPeerSyncEvents) < webUIMaxPeerEvents {
		webUIPeerSyncEvents = append(webUIPeerSyncEvents, event)
	} else {
		webUIPeerSyncEvents[webUIPeerSyncEventStart] = event
		webUIPeerSyncEventStart = (webUIPeerSyncEventStart + 1) % webUIMaxPeerEvents
	}
	webUIPeerSyncMutex.Unlock()
}

func WebUI_RecordBlockPeerAdded(peerIP string) {
	if !ConfigSnapshot().WebUI {
		return
	}
	peer, exists := GetWebUIBlockPeer(peerIP)
	if exists {
		AppendWebUIBlockPeerEvent(webUIBlockPeerEvent{Peer: &peer})
	}
}

func WebUI_RecordBlockPeerRemoved(peerIP string) {
	if !ConfigSnapshot().WebUI {
		return
	}
	AppendWebUIBlockPeerEvent(webUIBlockPeerEvent{RemovedIP: peerIP})
}

func WebUIBlockPeerEventAt(index int) webUIBlockPeerEvent {
	return webUIPeerSyncEvents[(webUIPeerSyncEventStart+index)%len(webUIPeerSyncEvents)]
}

func FullWebUIBlockPeerSyncLocked() WebUIBlockPeerSyncResponse {
	return WebUIBlockPeerSyncResponse{
		Epoch: webUIPeerSyncEpoch, Reset: true,
		Cursor:    webUIPeerSyncCursor,
		Peers:     GetWebUIBlockPeers(),
		RemovedIP: []string{},
	}
}

func GetWebUIBlockPeerSync(cursorValue string, epochs ...string) WebUIBlockPeerSyncResponse {
	webUIPeerSyncMutex.Lock()
	defer webUIPeerSyncMutex.Unlock()

	if cursorValue == "" || (len(epochs) > 0 && epochs[0] != webUIPeerSyncEpoch) {
		return FullWebUIBlockPeerSyncLocked()
	}
	cursor, err := strconv.ParseUint(cursorValue, 10, 64)
	if err != nil || cursor > webUIPeerSyncCursor {
		return FullWebUIBlockPeerSyncLocked()
	}
	if len(webUIPeerSyncEvents) == 0 {
		if cursor != webUIPeerSyncCursor {
			return FullWebUIBlockPeerSyncLocked()
		}
		return WebUIBlockPeerSyncResponse{Epoch: webUIPeerSyncEpoch, Cursor: webUIPeerSyncCursor, Peers: []WebUIBlockPeer{}, RemovedIP: []string{}}
	}
	if cursor+1 < WebUIBlockPeerEventAt(0).Cursor {
		return FullWebUIBlockPeerSyncLocked()
	}

	upserts := make(map[string]WebUIBlockPeer)
	removed := make(map[string]struct{})
	for index := range webUIPeerSyncEvents {
		event := WebUIBlockPeerEventAt(index)
		if event.Cursor <= cursor {
			continue
		}
		if event.Peer != nil {
			upserts[event.Peer.IP] = *event.Peer
			delete(removed, event.Peer.IP)
		} else {
			delete(upserts, event.RemovedIP)
			removed[event.RemovedIP] = struct{}{}
		}
	}

	peers := make([]WebUIBlockPeer, 0, len(upserts))
	for _, peer := range upserts {
		peers = append(peers, peer)
	}
	sort.Slice(peers, func(i, j int) bool {
		if peers[i].Timestamp == peers[j].Timestamp {
			return peers[i].IP < peers[j].IP
		}
		return peers[i].Timestamp > peers[j].Timestamp
	})
	removedIPs := make([]string, 0, len(removed))
	for peerIP := range removed {
		removedIPs = append(removedIPs, peerIP)
	}
	sort.Strings(removedIPs)

	return WebUIBlockPeerSyncResponse{
		Epoch: webUIPeerSyncEpoch, Cursor: webUIPeerSyncCursor,
		Peers:     peers,
		RemovedIP: removedIPs,
	}
}

func WebUI_GetStatus(w http.ResponseWriter, r *http.Request) {
	loadedExtensions := []string{}
	if ConfigSnapshot().SyncServerURL != "" {
		loadedExtensions = append(loadedExtensions, "SyncServer")
	}
	if currentBTN, _, _ := BtnSnapshot(); currentBTN != nil {
		loadedExtensions = append(loadedExtensions, "BTN")
	}

	totalBlockedIPs, totalBlockedPorts := GetWebUIBlockStats()
	stats := Stats{
		TotalBlockedIPs:     totalBlockedIPs,
		TotalBlockedPorts:   totalBlockedPorts,
		LastUpdateTimestamp: atomic.LoadInt64(&currentTimestamp),
	}

	resp := StatusResponse{
		ProgramName:      programName,
		ProgramVersion:   programVersion,
		UptimeSeconds:    time.Now().Unix() - programStartTimestamp,
		ClientType:       CurrentClientTypeSnapshot(),
		ClientURL:        ConfigSnapshot().ClientURL,
		LoadedExtensions: loadedExtensions,
		CurrentStats:     stats,
		Runtime: Runtime{
			GoVersion:    runtime.Version(),
			NumGoroutine: runtime.NumGoroutine(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func WebUI_GetPeers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, syncRequested := r.URL.Query()["sync"]; syncRequested {
		epochs := r.URL.Query()["epoch"]
		json.NewEncoder(w).Encode(GetWebUIBlockPeerSync(r.URL.Query().Get("cursor"), epochs...))
		return
	}
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
