package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestHTTPReloadDuringRequests(t *testing.T) {
	oldConfig := ConfigSnapshot()
	oldTransport, oldClient, oldExternal := httpTransport, httpClient, httpClientExternal
	t.Cleanup(func() {
		ReplaceConfig(oldConfig)
		httpTransport, httpClient, httpClientExternal = oldTransport, oldClient, oldExternal
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer server.Close()
	cfg := *oldConfig
	cfg.Proxy, cfg.LogPath = "", ""
	cfg.LogToFile, cfg.WebUI = false, false
	cfg.Timeout, cfg.LongConnection = 2, true
	ReplaceConfig(&cfg)
	InitConfig()
	firstTransport := httpTransport
	var workers sync.WaitGroup
	for n := 0; n < 4; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 40; i++ {
				code, _, body := Fetch(server.URL, false, i%2 == 0, false, nil)
				if code != 200 || string(body) != "ok" {
					t.Errorf("request during reload: status=%d body=%q", code, body)
					return
				}
			}
		}()
	}
	for i := 0; i < 20; i++ {
		UpdateConfig(func(c *ConfigStruct) { c.LongConnection = i%2 == 0 })
		InitConfig()
	}
	workers.Wait()
	if firstTransport.DisableKeepAlives {
		t.Fatal("reload mutated the old transport")
	}
	if !httpTransport.DisableKeepAlives {
		t.Fatal("reload did not disable keep-alive")
	}
}

func TestBTNDisabledReloadDuringReads(t *testing.T) {
	oldConfig := ConfigSnapshot()
	oldBTN, oldRules, oldExceptions := BtnSnapshot()
	t.Cleanup(func() { ReplaceConfig(oldConfig); btnConfig, btnRules, btnExceptions = oldBTN, oldRules, oldExceptions })
	cfg := *oldConfig
	cfg.BTNConfigureURL = ""
	ReplaceConfig(&cfg)
	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		for i := 0; i < 1000; i++ {
			BTN_CheckPeer("192.0.2.1", "peer", "client", 6881)
			BtnSnapshot()
		}
	}()
	for i := 0; i < 1000; i++ {
		BTN_GetConfig()
	}
	readers.Wait()
}
