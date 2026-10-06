package app

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestBTNConfigCannotReturnAfterDisable(t *testing.T) {
	oldCfg := ConfigSnapshot()
	oldBTN, oldRules, oldExceptions := BtnSnapshot()
	oldHTTP, oldExternal, oldTransport := httpClient, httpClientExternal, httpTransport
	oldNow, oldLast := currentTimestamp, atomic.LoadInt64(&btn_lastGetConfig)
	t.Cleanup(func() {
		ReplaceConfig(oldCfg)
		btnConfig, btnRules, btnExceptions = oldBTN, oldRules, oldExceptions
		httpClient, httpClientExternal, httpTransport = oldHTTP, oldExternal, oldTransport
		currentTimestamp = oldNow
		atomic.StoreInt64(&btn_lastGetConfig, oldLast)
	})
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Write([]byte(`{"min_protocol_version":1,"max_protocol_version":3,"ability":{}}`))
	}))
	defer server.Close()
	cfg := *oldCfg
	cfg.BTNConfigureURL = server.URL
	ReplaceConfig(&cfg)
	currentTimestamp = 100
	atomic.StoreInt64(&btn_lastGetConfig, 0)
	httpClientExternal = *server.Client()
	done := make(chan struct{})
	go func() { defer close(done); BTN_GetConfig() }()
	<-started
	UpdateConfig(func(c *ConfigStruct) { c.BTNConfigureURL = "" })
	InitConfig()
	close(release)
	<-done
	if current, _, _ := BtnSnapshot(); current != nil {
		t.Fatal("disabled BTN was re-enabled by an old response")
	}
}
