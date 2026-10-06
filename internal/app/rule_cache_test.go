package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	rulepkg "github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/app/rules"
)

func SetupRuleCacheTest(t *testing.T, isIP bool) (*ConfigStruct, *sync.Map, func() bool) {
	t.Helper()
	oldConfig := ConfigSnapshot()
	oldTransport, oldInternal, oldExternal := httpTransport, httpClient, httpClientExternal
	oldNow, oldBlockFetch, oldIPFetch := currentTimestamp, blockListURLLastFetch, ipBlockListURLLastFetch
	oldStore, oldEntries := ruleStore, remoteRuleEntries
	oldETags, oldLastModified := urlETagCache, urlLastModCache
	oldBlockMods, oldIPMods := blockListFileLastMod, ipBlockListFileLastMod
	t.Cleanup(func() {
		ReplaceConfig(oldConfig)
		httpTransport, httpClient, httpClientExternal = oldTransport, oldInternal, oldExternal
		currentTimestamp, blockListURLLastFetch, ipBlockListURLLastFetch = oldNow, oldBlockFetch, oldIPFetch
		ruleStore, remoteRuleEntries = oldStore, oldEntries
		urlETagCache, urlLastModCache = oldETags, oldLastModified
		blockListFileLastMod, ipBlockListFileLastMod = oldBlockMods, oldIPMods

	})
	ruleStore = NewRuleStore()
	cfg := *oldConfig
	cfg.BlockList, cfg.BlockListURL, cfg.BlockListFile = nil, nil, nil
	cfg.IPBlockList, cfg.IPBlockListURL, cfg.IPBlockListFile = nil, nil, nil
	cfg.RuleCachePath, cfg.Proxy, cfg.LogPath = t.TempDir(), "", ""
	cfg.LogToFile, cfg.WebUI = false, false
	cfg.UpdateInterval, cfg.Timeout = 10, 2
	ReplaceConfig(&cfg)
	currentTimestamp = 1000
	remoteRuleEntries = make(map[rulepkg.Source]rulepkg.CacheEntry)
	InitConfig()
	if isIP {
		return &cfg, &ruleStore.IPBlockList, SetIPBlockListFromURL
	}
	return &cfg, &ruleStore.BlockList, SetBlockListFromURL
}

func AssertRulePresent(t *testing.T, compiled *sync.Map, rule string, present bool) {
	t.Helper()
	if _, exists := compiled.Load(rule); exists != present {
		t.Fatalf("rule %q present=%t, want %t", rule, exists, present)
	}
}

func TestRemoteRuleCacheRestartOffline(t *testing.T) {
	for _, isIP := range []bool{false, true} {
		t.Run(map[bool]string{false: "client", true: "ip"}[isIP], func(t *testing.T) {
			cfg, compiled, update := SetupRuleCacheTest(t, isIP)
			rule := "CachedAgent"
			if isIP {
				rule = "192.0.2.0/24"
			}
			calls, offline := 0, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls > 1 && (r.Header.Get("If-None-Match") != "v1" || r.Header.Get("If-Modified-Since") == "") {
					t.Error("restart did not restore HTTP validators")
				}
				if offline {
					w.Header().Set("ETag", "unavailable")
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if r.Header.Get("If-None-Match") == "v1" {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("ETag", "v1")
				w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
				json.NewEncoder(w).Encode([]string{rule})
			}))
			defer server.Close()
			if isIP {
				cfg.IPBlockListURL = []string{server.URL}
			} else {
				cfg.BlockListURL = []string{server.URL}
			}
			ReplaceConfig(cfg)
			InitConfig()
			if !update() {
				t.Fatal("initial update failed")
			}
			AssertRulePresent(t, compiled, rule, true)
			source := rulepkg.Source{IP: isIP, Kind: "url", ID: server.URL}
			filename := rulepkg.CacheFilename(cfg.RuleCachePath, source)
			before, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			remoteRuleEntries = make(map[rulepkg.Source]rulepkg.CacheEntry)
			offline = true
			InitConfig()
			AssertRulePresent(t, compiled, rule, true)
			if calls != 1 {
				t.Fatal("cache restoration made a network request")
			}
			update()
			AssertRulePresent(t, compiled, rule, true)
			after, _ := os.ReadFile(filename)
			if string(before) != string(after) {
				t.Fatal("failed update overwrote the cache")
			}
			offline = false
			currentTimestamp += 11
			update()
			entry, err := rulepkg.ReadCache(cfg.RuleCachePath, source)
			if err != nil || entry.UpdatedAt != currentTimestamp || entry.ETag != "v1" {
				t.Fatalf("304 did not preserve and revalidate cached content: %+v, %v", entry, err)
			}
			remoteRuleEntries = make(map[rulepkg.Source]rulepkg.CacheEntry)
			InitConfig()
			AssertRulePresent(t, compiled, rule, true)
		})
	}
}

func TestRemoteRuleSourceReplacement(t *testing.T) {
	for _, isIP := range []bool{false, true} {
		t.Run(map[bool]string{false: "client", true: "ip"}[isIP], func(t *testing.T) {
			cfg, compiled, update := SetupRuleCacheTest(t, isIP)
			old, next, shared, local, other := "OldAgent", "NewAgent", "SharedAgent", "LocalAgent", "OtherAgent"
			if isIP {
				old, next, shared, local, other = "192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4", "192.0.2.5"
			}
			first := []string{old, shared, local, other}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/first" {
					json.NewEncoder(w).Encode(first)
				} else {
					json.NewEncoder(w).Encode([]string{other})
				}
			}))
			defer server.Close()
			localFile := filepath.Join(t.TempDir(), "local.txt")
			if err := os.WriteFile(localFile, []byte(local+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if isIP {
				cfg.IPBlockList, cfg.IPBlockListFile = []string{shared}, []string{localFile}
				cfg.IPBlockListURL = []string{server.URL + "/first", server.URL + "/second"}
			} else {
				cfg.BlockList, cfg.BlockListFile = []string{shared}, []string{localFile}
				cfg.BlockListURL = []string{server.URL + "/first", server.URL + "/second"}
			}
			ReplaceConfig(cfg)
			InitConfig()
			SetBlockListFromFile()
			SetIPBlockListFromFile()
			update()
			first = []string{next}
			currentTimestamp += 11
			update()
			AssertRulePresent(t, compiled, old, false)
			for _, rule := range []string{next, shared, local, other} {
				AssertRulePresent(t, compiled, rule, true)
			}
			first = []string{}
			currentTimestamp += 11
			update()
			AssertRulePresent(t, compiled, next, false)
			if isIP {
				cfg.IPBlockListURL = []string{server.URL + "/first"}
			} else {
				cfg.BlockListURL = []string{server.URL + "/first"}
			}
			ReplaceConfig(cfg)
			InitConfig()
			SetBlockListFromFile()
			SetIPBlockListFromFile()
			AssertRulePresent(t, compiled, other, false)
			AssertRulePresent(t, compiled, shared, true)
			AssertRulePresent(t, compiled, local, true)
		})
	}
}

func TestRemoteRuleCacheRejectsInvalidUpdate(t *testing.T) {
	cases := []struct {
		name, contentType, body string
		isIP                    bool
	}{
		{"json", "application/json", `["broken"`, false},
		{"null", "application/json", `null`, false},
		{"regexp", "text/plain", "[", false},
		{"ip", "text/plain", "invalid-ip", true},
		{"html", "text/plain", "<!DOCTYPE HTML><html>Unavailable</html>", false},
		{"large", "text/plain", strings.Repeat("x", rulepkg.MaxContentSize+1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, compiled, update := SetupRuleCacheTest(t, tc.isIP)
			rule := "ValidAgent"
			if tc.isIP {
				rule = "198.51.100.1"
			}
			invalid := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if invalid {
					if r.Header.Get("If-None-Match") != "good" {
						t.Error("invalid response replaced the accepted validator")
					}
					w.Header().Set("Content-Type", tc.contentType)
					w.Header().Set("ETag", "bad")
					w.Write([]byte(tc.body))
					return
				}
				w.Header().Set("Content-Type", "text/plain")
				w.Header().Set("ETag", "good")
				w.Write([]byte(rule))
			}))
			defer server.Close()
			if tc.isIP {
				cfg.IPBlockListURL = []string{server.URL}
			} else {
				cfg.BlockListURL = []string{server.URL}
			}
			ReplaceConfig(cfg)
			InitConfig()
			update()
			filename := rulepkg.CacheFilename(cfg.RuleCachePath, rulepkg.Source{IP: tc.isIP, Kind: "url", ID: server.URL})
			before, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			invalid = true
			for attempt := 0; attempt < 2; attempt++ {
				currentTimestamp += 11
				update()
				AssertRulePresent(t, compiled, rule, true)
			}
			after, _ := os.ReadFile(filename)
			if string(before) != string(after) {
				t.Fatal("invalid update replaced the last valid disk cache")
			}
			remoteRuleEntries = make(map[rulepkg.Source]rulepkg.CacheEntry)
			InitConfig()
			AssertRulePresent(t, compiled, rule, true)
		})
	}
}

func TestRemoteRuleCacheUnavailableOrDisabled(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "unwritable", true: "disabled"}[disabled], func(t *testing.T) {
			cfg, compiled, update := SetupRuleCacheTest(t, false)
			if disabled {
				cfg.RuleCachePath = ""
			} else if err := os.WriteFile(filepath.Join(cfg.RuleCachePath, "blocked"), []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			} else {
				cfg.RuleCachePath = filepath.Join(cfg.RuleCachePath, "blocked")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("MemoryAgent"))
			}))
			defer server.Close()
			cfg.BlockListURL = []string{server.URL}
			ReplaceConfig(cfg)
			InitConfig()
			update()
			AssertRulePresent(t, compiled, "MemoryAgent", true)
			InitConfig()
			AssertRulePresent(t, compiled, "MemoryAgent", true)
		})
	}
}

func TestRemoteRuleCacheCorruptionRequiresFullFetch(t *testing.T) {
	for _, corrupt := range []string{"json", "metadata", "rules"} {
		t.Run(corrupt, func(t *testing.T) {
			cfg, compiled, update := SetupRuleCacheTest(t, false)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
					t.Error("corrupt cache sent a conditional request")
				}
				w.Write([]byte("FreshAgent"))
			}))
			defer server.Close()
			cfg.BlockListURL = []string{server.URL}
			source := rulepkg.Source{Kind: "url", ID: server.URL}
			entry := rulepkg.CacheEntry{Version: 1, URL: server.URL, Body: []byte("CachedAgent"), ETag: "stale", UpdatedAt: 900}
			if corrupt == "metadata" {
				entry.URL = "http://another-source.invalid"
			} else if corrupt == "rules" {
				entry.Body = []byte("[")
			}
			if err := rulepkg.WriteCache(cfg.RuleCachePath, source, entry); err != nil {
				t.Fatal(err)
			}
			if corrupt == "json" {
				if err := os.WriteFile(rulepkg.CacheFilename(cfg.RuleCachePath, source), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			ReplaceConfig(cfg)
			InitConfig()
			AssertRulePresent(t, compiled, "CachedAgent", false)
			update()
			AssertRulePresent(t, compiled, "FreshAgent", true)
		})
	}
}

func TestRuleCacheConfigFormats(t *testing.T) {
	for _, format := range []string{"json", "toml"} {
		for _, path := range []string{"custom/rules", ""} {
			filename := filepath.Join(t.TempDir(), "config."+format)
			content := `{"ruleCachePath":"` + path + `"}`
			if format == "toml" {
				content = `ruleCachePath = "` + path + `"`
			}
			if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg := ConfigStruct{RuleCachePath: "cache/rules"}
			if status := LoadConfig(filename, true, &cfg); status != 0 || cfg.RuleCachePath != path {
				t.Fatalf("%s cache path=%q, want %q (status=%d)", format, cfg.RuleCachePath, path, status)
			}
			t.Cleanup(func() {
				lastModMutex.Lock()
				delete(configLastMod, filename)
				lastModMutex.Unlock()
			})
		}
	}
}

func TestRemoteRuleCacheRejectsUnsolicited304(t *testing.T) {
	cfg, compiled, update := SetupRuleCacheTest(t, false)
	unchanged := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unchanged {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Write([]byte("SavedAgent"))
	}))
	defer server.Close()
	cfg.BlockListURL = []string{server.URL}
	ReplaceConfig(cfg)
	InitConfig()
	update()
	filename := rulepkg.CacheFilename(cfg.RuleCachePath, rulepkg.Source{Kind: "url", ID: server.URL})
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	unchanged = true
	currentTimestamp += 11
	update()
	AssertRulePresent(t, compiled, "SavedAgent", true)
	after, err := os.ReadFile(filename)
	if err != nil || string(after) != string(before) {
		t.Fatalf("an unsolicited 304 revalidated the cache: %v", err)
	}
}
