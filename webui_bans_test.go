package main

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestWebUIBanPagination(t *testing.T) {
	InstallScreenshotScalePeers(t, map[string]BlockPeerInfoStruct{
		"192.0.2.1":   {Timestamp: 10, Module: "A", Reason: "Rule", Port: map[int]bool{80: true}},
		"192.0.2.2":   {Timestamp: 20, Module: "A", Reason: "Rule", Port: map[int]bool{81: true}},
		"2001:db8::1": {Timestamp: 30, Module: "B", Port: map[int]bool{-1: true}},
	})
	ResetWebUIPeerSync()
	get := func(query string) (WebUIBanPage, int) {
		rec := httptest.NewRecorder()
		WebUI_GetBans(rec, httptest.NewRequest("GET", "/api/v1/bans?"+query, nil))
		var p WebUIBanPage
		json.Unmarshal(rec.Body.Bytes(), &p)
		return p, rec.Code
	}
	first, code := get("limit=1&module=A")
	if code != 200 || first.Total != 3 || first.FilteredTotal != 2 || len(first.Items) != 1 || first.Items[0].IP != "192.0.2.2" || first.NextCursor == "" {
		t.Fatalf("first: %+v %d", first, code)
	}
	blockPeerMapMutex.Lock()
	delete(blockPeerMap, "192.0.2.1")
	blockPeerMapMutex.Unlock()
	second, code := get("limit=1&module=A&cursor=" + url.QueryEscape(first.NextCursor))
	if code != 200 || len(second.Items) != 1 || second.Items[0].IP != "192.0.2.1" || second.NextCursor != "" || second.Offset != 1 {
		t.Fatalf("unstable snapshot: %+v %d", second, code)
	}
	for _, query := range []string{"limit=0", "limit=201", "sort=bad", "from=30&to=10", "from=bad", "cursor=bad", "limit=1&module=B&cursor=" + url.QueryEscape(first.NextCursor)} {
		if _, code := get(query); code != 400 {
			t.Fatalf("invalid query accepted: %s (%d)", query, code)
		}
	}
	filtered, code := get("query=ALL&sort=ip&from=25")
	if code != 200 || len(filtered.Items) != 1 || filtered.Items[0].IP != "2001:db8::1" {
		t.Fatalf("filter: %+v", filtered)
	}
	rec := httptest.NewRecorder()
	WebUI_GetBan(rec, httptest.NewRequest("GET", "/api/v1/bans/2001:db8::1", nil))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec = httptest.NewRecorder()
	WebUI_GetBan(rec, httptest.NewRequest("GET", "/api/v1/bans/192.0.2.1", nil))
	if rec.Code != 404 {
		t.Fatal("detail returned deleted record")
	}
	webUIBanSnapshotMutex.Lock()
	webUIBanSnapshot.Created = time.Now().Add(-time.Minute)
	webUIBanSnapshotMutex.Unlock()
	if _, code := get("limit=1&module=A&cursor=" + url.QueryEscape(first.NextCursor)); code != 409 {
		t.Fatalf("expired cursor: %d", code)
	}
}
