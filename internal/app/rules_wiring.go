package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	rulepkg "github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/app/rules"
	"github.com/tidwall/jsonc"
)

var ruleReloadMutex sync.Mutex
var ruleGeneration uint64
var blockListURLFetching atomic.Bool
var ipBlockListURLFetching atomic.Bool

var blockListURLLastFetch int64 = 0
var ipBlockListURLLastFetch int64 = 0

// blockListFileLastMod 记录黑名单文件的最后修改时间, 用于热重载判断.
var blockListFileLastMod = make(map[string]int64)

// ipBlockListFileLastMod 记录 IP 黑名单文件的最后修改时间.
var ipBlockListFileLastMod = make(map[string]int64)

// lastModMutex 保护本地配置与黑名单文件的修改时间映射.
var lastModMutex sync.RWMutex

func NewRuleStore() *rulepkg.Store {
	return rulepkg.NewStore(rulepkg.Services{Log: Log, LogError: LogError, Text: GetLangText})
}

var ruleStore = NewRuleStore()

// remoteRuleEntries 由 ruleReloadMutex 保护, 仅保留已通过校验的响应.
var remoteRuleEntries = make(map[rulepkg.Source]rulepkg.CacheEntry)

// 调用时持有 ruleReloadMutex. 只恢复当前配置仍订阅的 URL.
func RestoreRuleCaches(cfg *ConfigStruct) {
	previous := remoteRuleEntries
	remoteRuleEntries = make(map[rulepkg.Source]rulepkg.CacheEntry)
	for _, isIP := range []bool{false, true} {
		urls := cfg.BlockListURL
		if isIP {
			urls = cfg.IPBlockListURL
		}
		for _, url := range urls {
			source := rulepkg.Source{IP: isIP, Kind: "url", ID: url}
			entry, exists := previous[source]
			if !exists {
				if cfg.RuleCachePath == "" {
					continue
				}
				var err error
				entry, err = rulepkg.ReadCache(cfg.RuleCachePath, source)
				if err != nil {
					if !os.IsNotExist(err) {
						LogError("RuleCache", GetLangText("Error-RuleCacheRead"), true, url, err)
					}
					continue
				}
			}
			content, err := rulepkg.ParseRemoteRuleContent(entry)
			if err != nil {
				LogError("RuleCache", GetLangText("Error-RemoteRulesInvalid"), true, url, err)
				continue
			}
			rules, err := ruleStore.CompileRuleContent(content, source, true)
			if err != nil {
				continue
			}
			ruleStore.PublishRuleSource(source, rules)
			remoteRuleEntries[source] = entry
			Log("RuleCache", GetLangText("Success-RuleCacheLoaded"), true, len(rules), url, time.Unix(entry.UpdatedAt, 0).Format(time.RFC3339))
		}
	}
}

func SetRulesFromURL(isIP bool) bool {
	fetching, lastFetch := &blockListURLFetching, &blockListURLLastFetch
	module := "SetBlockListFromURL"
	if isIP {
		fetching, lastFetch, module = &ipBlockListURLFetching, &ipBlockListURLLastFetch, "SetIPBlockListFromURL"
	}
	if !fetching.CompareAndSwap(false, true) {
		return true
	}
	defer fetching.Store(false)
	ruleReloadMutex.Lock()
	cfg, generation := ConfigSnapshot(), ruleGeneration
	now := atomic.LoadInt64(&currentTimestamp)
	urls := cfg.BlockListURL
	if isIP {
		urls = cfg.IPBlockListURL
	}
	if len(urls) == 0 || (*lastFetch+int64(cfg.UpdateInterval)) > now {
		ruleReloadMutex.Unlock()
		return true
	}
	*lastFetch = now
	ruleReloadMutex.Unlock()
	setCount := 0
	for _, url := range urls {
		source := rulepkg.Source{IP: isIP, Kind: "url", ID: url}
		ruleReloadMutex.Lock()
		if generation != ruleGeneration {
			ruleReloadMutex.Unlock()
			return false
		}
		previous, exists := remoteRuleEntries[source]
		ruleReloadMutex.Unlock()
		headers := map[string]string{}
		if exists {
			if previous.ETag != "" {
				headers["If-None-Match"] = previous.ETag
			}
			if previous.LastModified != "" {
				headers["If-Modified-Since"] = previous.LastModified
			}
		}
		// 校验器必须与已接受的规则绑定, 不能使用 Fetch 的通用内存缓存.
		status, responseHeaders, body := Fetch(url, false, false, false, &headers)
		entry := rulepkg.CacheEntry{Version: 1, URL: url, IP: isIP, Body: body, ContentType: responseHeaders.Get("Content-Type"), ETag: responseHeaders.Get("ETag"), LastModified: responseHeaders.Get("Last-Modified"), UpdatedAt: now}
		var rules map[string]interface{}
		var err error
		if status == 304 && exists && len(headers) > 0 {
			entry = previous
			entry.UpdatedAt = now
			if etag := responseHeaders.Get("ETag"); etag != "" {
				entry.ETag = etag
			}
			if lastModified := responseHeaders.Get("Last-Modified"); lastModified != "" {
				entry.LastModified = lastModified
			}
		} else if status == 200 && body != nil {
			var content []string
			content, err = rulepkg.ParseRemoteRuleContent(entry)
			if err == nil {
				rules, err = ruleStore.CompileRuleContent(content, source, true)
			}
		} else {
			err = fmt.Errorf("HTTP status %d", status)
		}
		ruleReloadMutex.Lock()
		if generation != ruleGeneration {
			ruleReloadMutex.Unlock()
			return false
		}
		if err != nil {
			LogError(module, GetLangText("Error-RemoteRulesInvalid"), true, url, err)
			if exists {
				Log(module, GetLangText("RuleCache-Fallback"), true, url, time.Unix(previous.UpdatedAt, 0).Format(time.RFC3339))
			}
			ruleReloadMutex.Unlock()
			continue
		}
		if err := rulepkg.WriteCache(cfg.RuleCachePath, source, entry); err != nil {
			LogError("RuleCache", GetLangText("Error-RuleCacheWrite"), true, url, err)
		}
		if rules != nil {
			setCount += ruleStore.PublishRuleSource(source, rules)
		}
		remoteRuleEntries[source] = entry
		ruleReloadMutex.Unlock()
	}
	Log(module, GetLangText("Success-"+module), true, setCount)
	return true
}

func SetBlockListFromContent(blockListContent []string, blockListSource string) int {
	return ruleStore.SetRuleContent(blockListContent, rulepkg.Source{Kind: "content", ID: blockListSource})
}
func SetBlockListFromFile() bool {
	ruleReloadMutex.Lock()
	defer ruleReloadMutex.Unlock()
	if len(ConfigSnapshot().BlockListFile) == 0 {
		return true
	}

	setCount := 0
	updated := false

	for _, filePath := range ConfigSnapshot().BlockListFile {
		blockListFileStat, err := os.Stat(filePath)
		if err != nil {
			LogError("SetBlockListFromFile", GetLangText("Error-LoadFile"), false, filePath, err.Error())
			return false
		}

		// 最大 8 MB.
		if blockListFileStat.Size() > 8388608 {
			LogError("SetBlockListFromFile", GetLangText("Error-LargeFile"), true)
			continue
		}

		// 获取当前文件的最后修改时间.
		fileLastMod := blockListFileStat.ModTime().Unix()
		// 为了线程安全, 先加读锁获取映射中的旧值到局部变量 lastMod.
		lastModMutex.RLock()
		lastMod := blockListFileLastMod[filePath]
		lastModMutex.RUnlock()

		// 如果文件未修改, 则跳过处理.
		if fileLastMod == lastMod {
			continue
		}
		if lastMod != 0 {
			Log("Debug-SetBlockListFromFile", GetLangText("Debug-SetBlockListFromFile_HotReload"), false, filePath)
		}

		blockListContent, err := os.ReadFile(filePath)
		if err != nil {
			LogError("SetBlockListFromFile", GetLangText("Error-LoadFile"), true, filePath, err.Error())
			return false
		}

		var content []string
		if filepath.Ext(filePath) == ".json" {
			err = json.Unmarshal(jsonc.ToJSON(blockListContent), &content)
			if err != nil {
				LogError("SetBlockListFromFile", GetLangText("Error-GenJSONWithID"), true, filePath, err.Error())
				continue
			}
		} else {
			content = strings.Split(string(blockListContent), "\n")
		}

		setCount += ruleStore.SetRuleContent(content, rulepkg.Source{Kind: "file", ID: filePath})
		lastModMutex.Lock()
		blockListFileLastMod[filePath] = fileLastMod
		lastModMutex.Unlock()
		updated = true
	}

	if updated {
		Log("SetBlockListFromFile", GetLangText("Success-SetBlockListFromFile"), true, setCount)
	}
	return true
}
func SetBlockListFromURL() bool {
	return SetRulesFromURL(false)
}
func SetIPBlockListFromContent(ipBlockListContent []string, ipBlockListSource string) int {
	return ruleStore.SetRuleContent(ipBlockListContent, rulepkg.Source{IP: true, Kind: "content", ID: ipBlockListSource})
}
func SetIPBlockListFromFile() bool {
	ruleReloadMutex.Lock()
	defer ruleReloadMutex.Unlock()
	if len(ConfigSnapshot().IPBlockListFile) == 0 {
		return true
	}

	setCount := 0
	updated := false

	for _, filePath := range ConfigSnapshot().IPBlockListFile {
		ipBlockListFileStat, err := os.Stat(filePath)
		if err != nil {
			LogError("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), false, filePath, err.Error())
			return false
		}

		// 获取当前文件的最后修改时间.
		fileLastMod := ipBlockListFileStat.ModTime().Unix()
		// 加读锁获取旧值.
		lastModMutex.RLock()
		lastMod := ipBlockListFileLastMod[filePath]
		lastModMutex.RUnlock()

		if fileLastMod <= lastMod {
			continue
		}

		if lastMod != 0 {
			Log("Debug-SetIPBlockListFromFile", GetLangText("Debug-SetIPBlockListFromFile_HotReload"), false, filePath)
		}

		ipBlockListFile, err := os.ReadFile(filePath)
		if err != nil {
			LogError("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), true, filePath, err.Error())
			return false
		}

		var content []string
		if filepath.Ext(filePath) == ".json" {
			err := json.Unmarshal(jsonc.ToJSON(ipBlockListFile), &content)
			if err != nil {
				LogError("SetIPBlockListFromFile", GetLangText("Error-GenJSONWithID"), true, filePath, err.Error())
				continue
			}
		} else {
			content = strings.Split(string(ipBlockListFile), "\n")
		}

		setCount += ruleStore.SetRuleContent(content, rulepkg.Source{IP: true, Kind: "file", ID: filePath})
		lastModMutex.Lock()
		ipBlockListFileLastMod[filePath] = fileLastMod
		lastModMutex.Unlock()
		updated = true
	}

	if updated {
		Log("SetIPBlockListFromFile", GetLangText("Success-SetIPBlockListFromFile"), true, setCount)
	}
	return true
}
func SetIPBlockListFromURL() bool {
	return SetRulesFromURL(true)
}
