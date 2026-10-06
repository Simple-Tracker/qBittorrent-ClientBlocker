package app

import (
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/webui"
)

func init() {
	webui.Configure(webui.Dependencies{
		Config: func() webui.Config {
			cfg := ConfigSnapshot()
			return webui.Config{Enabled: cfg.WebUI, Username: cfg.WebUIUsername, Password: cfg.WebUIPassword}
		},
		Status:     webUIStatus,
		BlockStats: webUIBlockStats,
		BlockPeers: func() []webui.WebUIBlockPeer {
			blockPeerMapMutex.RLock()
			defer blockPeerMapMutex.RUnlock()
			peers := make([]webui.WebUIBlockPeer, 0, len(blockPeerMap))
			for ip, peer := range blockPeerMap {
				peers = append(peers, webUIBlockPeer(ip, peer))
			}
			return peers
		},
		BlockPeer: func(ip string) (webui.WebUIBlockPeer, bool) {
			blockPeerMapMutex.RLock()
			defer blockPeerMapMutex.RUnlock()
			peer, exists := blockPeerMap[ip]
			if !exists {
				return webui.WebUIBlockPeer{}, false
			}
			return webUIBlockPeer(ip, peer), true
		},
		LegacyLogs: func() []string { return appLogger.LegacyLogs() },
		Submission: func() webui.Submission {
			blockPeerMapMutex.RLock()
			defer blockPeerMapMutex.RUnlock()
			return webui.Submission{Pending: blockPeerSubmission.Pending, PendingIPs: len(blockPeerMap), NextRetry: blockPeerSubmission.Next}
		},
	})
}

func webUIBlockStats() (int, int) {
	blockPeerMapMutex.RLock()
	defer blockPeerMapMutex.RUnlock()
	ports := 0
	for _, peer := range blockPeerMap {
		ports += len(peer.Port)
	}
	return len(blockPeerMap), ports
}

func webUIBlockPeer(ip string, peer BlockPeerInfoStruct) webui.WebUIBlockPeer {
	ports := make([]int, 0, len(peer.Port))
	for port := range peer.Port {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	labels := make([]string, 0, len(ports))
	for _, port := range ports {
		if port == -1 {
			labels = append(labels, "ALL")
		} else {
			labels = append(labels, strconv.Itoa(port))
		}
	}
	return webui.WebUIBlockPeer{IP: ip, Timestamp: peer.Timestamp, Module: peer.Module, Reason: peer.Reason, Ports: labels, ID: peer.ID, Client: peer.Client, Downloaded: peer.Downloaded, Uploaded: peer.Uploaded}
}

func webUIStatus() webui.StatusResponse {
	cfg := ConfigSnapshot()
	extensions := []string{}
	if cfg.SyncServerURL != "" {
		extensions = append(extensions, "SyncServer")
	}
	if currentBTN, _, _ := BtnSnapshot(); currentBTN != nil {
		extensions = append(extensions, "BTN")
	}
	ips, ports := webUIBlockStats()
	return webui.StatusResponse{
		ProgramName: programName, ProgramVersion: programVersion,
		UptimeSeconds: time.Now().Unix() - programStartTimestamp,
		ClientType:    CurrentClientTypeSnapshot(), ClientURL: cfg.ClientURL, LoadedExtensions: extensions,
		CurrentStats: webui.Stats{TotalBlockedIPs: ips, TotalBlockedPorts: ports, LastUpdateTimestamp: atomic.LoadInt64(&currentTimestamp)},
		Runtime:      webui.Runtime{GoVersion: runtime.Version(), NumGoroutine: runtime.NumGoroutine()},
	}
}
