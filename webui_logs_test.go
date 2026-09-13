package main

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestStructuredLogCursorFilteringAndRetention(t *testing.T) {
	webUILogMutex.Lock()
	oldEntries, oldSequence, oldEpoch := webUILogEntries, webUILogSequence, webUILogEpoch
	webUILogSequence = 0
	webUILogEpoch = "test-logs"
	webUILogMutex.Unlock()
	t.Cleanup(func() {
		webUILogMutex.Lock()
		webUILogEntries, webUILogSequence, webUILogEpoch = oldEntries, oldSequence, oldEpoch
		webUILogMutex.Unlock()
	})
	get := func(query string) (WebUILogPage, int) {
		rec := httptest.NewRecorder()
		WebUI_GetStructuredLogs(rec, httptest.NewRequest("GET", "/api/v1/logs?"+query, nil))
		var p WebUILogPage
		json.Unmarshal(rec.Body.Bytes(), &p)
		return p, rec.Code
	}
	AppendWebUILog("info", "A", "hello")
	AppendWebUILog("error", "B", strings.Repeat("测", 2000))
	AppendWebUILog("info", "A", "third")
	first, code := get("limit=1&level=error")
	if code != 200 || !first.Reset || len(first.Items) != 1 || first.Items[0].Level != "error" || !first.Items[0].Truncated || !utf8.ValidString(first.Items[0].Message) || len(first.Items[0].Message) > 4096 {
		t.Fatalf("first: %+v", first)
	}
	next, code := get("level=error&after=" + url.QueryEscape(first.NextCursor))
	if code != 200 || next.Reset || len(next.Items) != 0 || next.HasMore {
		t.Fatalf("empty delta: %+v", next)
	}
	changed, _ := get("module=A&after=" + url.QueryEscape(next.NextCursor))
	if !changed.Reset || len(changed.Items) != 2 {
		t.Fatalf("filter reset: %+v", changed)
	}
	for i := 0; i < webUILogCapacity+2; i++ {
		AppendWebUILog("info", "A", "new")
	}
	expired, _ := get("after=" + url.QueryEscape(next.NextCursor))
	if !expired.Reset || len(expired.Items) != 100 {
		t.Fatalf("expired: %+v", expired)
	}
	for _, q := range []string{"after=bad", "limit=0", "limit=201", "level=unknown"} {
		if _, code := get(q); code != 400 {
			t.Fatalf("invalid %s: %d", q, code)
		}
	}
	webUILogMutex.Lock()
	webUILogEpoch = "restarted"
	webUILogMutex.Unlock()
	restarted, _ := get("after=" + url.QueryEscape(expired.NextCursor))
	if !restarted.Reset {
		t.Fatal("restart did not reset")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			AppendWebUILog("info", "Concurrent", "new")
		}
	}()
	for i := 0; i < 100; i++ {
		get("limit=10")
	}
	wg.Wait()
}
