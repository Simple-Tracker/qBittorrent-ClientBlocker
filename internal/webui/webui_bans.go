package webui

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type WebUIBanPage struct {
	Items         []WebUIBlockPeer `json:"items"`
	Total         int              `json:"total"`
	FilteredTotal int              `json:"filtered_total"`
	Offset        int              `json:"offset"`
	NextCursor    string           `json:"next_cursor"`
	SnapshotAt    int64            `json:"snapshot_at"`
	Modules       []string         `json:"modules"`
	Reasons       []string         `json:"reasons"`
}
type webUIBanCursor struct {
	Snapshot string
	Query    string
	Offset   int
}

var webUIBanSnapshotMutex sync.Mutex
var webUIBanSnapshot struct {
	ID, Epoch string
	Created   time.Time
	Peers     []WebUIBlockPeer
}

// 共用一份保留 30 秒的不可变快照, 避免保留量随调用方数量增长.
func WebUIBanPeersSnapshot() (string, int64, []WebUIBlockPeer) {
	webUIPeerSyncMutex.Lock()
	epoch := webUIPeerSyncEpoch
	webUIPeerSyncMutex.Unlock()
	webUIBanSnapshotMutex.Lock()
	defer webUIBanSnapshotMutex.Unlock()
	if webUIBanSnapshot.ID == "" || webUIBanSnapshot.Epoch != epoch || now().Sub(webUIBanSnapshot.Created) >= 30*time.Second {
		webUIBanSnapshot.Peers = GetWebUIBlockPeers()
		webUIBanSnapshot.Created = now()
		webUIBanSnapshot.ID = strconv.FormatInt(webUIBanSnapshot.Created.UnixNano(), 36)
		webUIBanSnapshot.Epoch = epoch
	}
	return webUIBanSnapshot.ID, webUIBanSnapshot.Created.Unix(), webUIBanSnapshot.Peers
}
func WebUI_GetBans(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	if value := q.Get("limit"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 200 {
			WriteWebUIAPIError(w, 400, "invalid_query")
			return
		}
		limit = n
	}
	order := q.Get("sort")
	if order == "" {
		order = "-timestamp"
	}
	switch order {
	case "timestamp", "-timestamp", "ip", "-ip", "uploaded", "-uploaded":
	default:
		WriteWebUIAPIError(w, 400, "invalid_query")
		return
	}
	var from, to int64
	for _, param := range []struct {
		name   string
		target *int64
	}{{"from", &from}, {"to", &to}} {
		if value := q.Get(param.name); value != "" {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 {
				WriteWebUIAPIError(w, 400, "invalid_query")
				return
			}
			*param.target = n
		}
	}
	if to > 0 && from > to {
		WriteWebUIAPIError(w, 400, "invalid_query")
		return
	}
	query := strings.ToLower(strings.TrimSpace(q.Get("query")))
	if len(query) > 256 || len(q.Get("module")) > 256 || len(q.Get("reason")) > 256 {
		WriteWebUIAPIError(w, 400, "invalid_query")
		return
	}
	binding, _ := json.Marshal([]interface{}{query, q.Get("module"), q.Get("reason"), from, to, order, limit})
	digest := fmt.Sprintf("%x", sha256.Sum256(binding))
	cursor := webUIBanCursor{}
	if raw := q.Get("cursor"); raw != "" {
		if len(raw) > 1024 {
			WriteWebUIAPIError(w, 400, "invalid_cursor")
			return
		}
		data, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Offset < 0 || cursor.Snapshot == "" || cursor.Query != digest {
			WriteWebUIAPIError(w, 400, "invalid_cursor")
			return
		}
	}
	id, created, peers := WebUIBanPeersSnapshot()
	if cursor.Snapshot != "" && cursor.Snapshot != id {
		WriteWebUIAPIError(w, 409, "snapshot_expired")
		return
	}
	indexes := []int{}
	modules, reasons := map[string]bool{}, map[string]bool{}
	for i, p := range peers {
		if p.Module != "" {
			modules[p.Module] = true
		}
		if p.Reason != "" {
			reasons[p.Reason] = true
		}
		if q.Get("module") != "" && p.Module != q.Get("module") || q.Get("reason") != "" && p.Reason != q.Get("reason") || from > 0 && p.Timestamp < from || to > 0 && p.Timestamp > to {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(strings.Join([]string{p.IP, p.Module, p.Reason, p.Client, p.ID, strings.Join(p.Ports, ",")}, " ")), query) {
			continue
		}
		indexes = append(indexes, i)
	}
	sort.Slice(indexes, func(i, j int) bool {
		a, b := peers[indexes[i]], peers[indexes[j]]
		switch order {
		case "ip":
			return a.IP < b.IP
		case "-ip":
			return a.IP > b.IP
		case "timestamp":
			if a.Timestamp != b.Timestamp {
				return a.Timestamp < b.Timestamp
			}
		case "-timestamp":
			if a.Timestamp != b.Timestamp {
				return a.Timestamp > b.Timestamp
			}
		case "uploaded":
			if a.Uploaded != b.Uploaded {
				return a.Uploaded < b.Uploaded
			}
		case "-uploaded":
			if a.Uploaded != b.Uploaded {
				return a.Uploaded > b.Uploaded
			}
		}
		return a.IP < b.IP
	})
	if cursor.Offset > len(indexes) {
		WriteWebUIAPIError(w, 400, "invalid_cursor")
		return
	}
	page := WebUIBanPage{Items: []WebUIBlockPeer{}, Total: len(peers), FilteredTotal: len(indexes), Offset: cursor.Offset, SnapshotAt: created, Modules: []string{}, Reasons: []string{}}
	end := cursor.Offset + limit
	if end > len(indexes) {
		end = len(indexes)
	}
	for _, i := range indexes[cursor.Offset:end] {
		page.Items = append(page.Items, peers[i])
	}
	if end < len(indexes) {
		data, _ := json.Marshal(webUIBanCursor{Snapshot: id, Query: digest, Offset: end})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(data)
	}
	for value := range modules {
		page.Modules = append(page.Modules, value)
	}
	for value := range reasons {
		page.Reasons = append(page.Reasons, value)
	}
	sort.Strings(page.Modules)
	sort.Strings(page.Reasons)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}
func WebUI_GetBan(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimPrefix(r.URL.Path, "/api/v1/bans/")
	if net.ParseIP(ip) == nil {
		WriteWebUIAPIError(w, 400, "invalid_ip")
		return
	}
	peer, ok := GetWebUIBlockPeer(ip)
	if !ok {
		WriteWebUIAPIError(w, 404, "not_found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peer)
}
