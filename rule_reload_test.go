package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuleReloadRejectsInFlightOldRules(t *testing.T) {
	oldCfg, oldClient, oldNow := ConfigSnapshot(), httpClientExternal, currentTimestamp
	oldInternal, oldTransport := httpClient, httpTransport
	t.Cleanup(func() {
		ReplaceConfig(oldCfg)
		httpClientExternal, currentTimestamp = oldClient, oldNow
		httpClient, httpTransport = oldInternal, oldTransport
		EraseSyncMap(&blockListCompiled)
	})
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("ETag", "old-generation")
		w.Write([]byte("obsolete-pattern"))
	}))
	defer server.Close()
	cfg := *oldCfg
	cfg.BlockList, cfg.BlockListURL = nil, []string{server.URL}
	cfg.UpdateInterval = 1
	ReplaceConfig(&cfg)
	currentTimestamp = 100
	ruleReloadMutex.Lock()
	blockListURLLastFetch = 0
	ruleReloadMutex.Unlock()
	httpClientExternal = *server.Client()
	done := make(chan bool, 1)
	go func() { done <- SetBlockListFromURL() }()
	<-started
	UpdateConfig(func(c *ConfigStruct) { c.BlockListURL = nil })
	InitConfig()
	close(release)
	if <-done {
		t.Fatal("stale rule fetch was published after reload")
	}
	count := 0
	blockListCompiled.Range(func(_, _ any) bool { count++; return true })
	if count != 0 {
		t.Fatal("old rules leaked into new configuration")
	}
	requestStateMutex.RLock()
	etag := urlETagCache[server.URL]
	requestStateMutex.RUnlock()
	if etag != "" {
		t.Fatal("discarded response left a stale HTTP validator")
	}
}
