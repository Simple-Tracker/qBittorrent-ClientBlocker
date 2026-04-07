package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
	"github.com/pelletier/go-toml/v2"
	"github.com/tidwall/jsonc"
)

type ConfigStruct struct {
	CheckUpdate                   bool
	Debug                         bool
	Debug_CheckTorrent            bool
	Debug_CheckPeer               bool
	Interval                      uint32
	CleanInterval                 uint32
	UpdateInterval                uint32
	RestartInterval               uint32
	TorrentMapCleanInterval       uint32
	BanTime                       uint32
	BanAllPort                    bool
	BanIPCIDR                     string
	BanIP6CIDR                    string
	IgnoreEmptyPeer               bool
	IgnoreNoLeechersTorrent       bool
	IgnorePTTorrent               bool
	IgnoreFailureExit             bool
	SleepTime                     uint32
	Timeout                       uint32
	Proxy                         string
	LongConnection                bool
	LogPath                       string
	LogToFile                     bool
	LogDebug                      bool
	Listen                        string
	WebUI                         bool
	WebUIListen                   string
	WebUIUsername                 string
	WebUIPassword                 string
	ClientType                    string
	ClientURL                     string
	ClientUsername                string
	ClientPassword                string
	UseBasicAuth                  bool
	UseShadowBan                  bool
	SkipCertVerification          bool
	FetchFailedThreshold          int
	ExecCommand_FetchFailed       string
	ExecCommand_Run               string
	ExecCommand_Ban               string
	ExecCommand_Unban             string
	SyncServerURL                 string
	SyncServerToken               string
	BTNConfigureURL               string
	BTNAppID                      string
	BTNAppSecret                  string
	BTNSyncRules                  bool
	BTNSyncException              bool
	BTNSubmitPeers                bool
	BTNSubmitBans                 bool
	BTNSubmitHistories            bool
	BlockList                     []string
	BlockListURL                  []string
	BlockListFile                 []string
	PortBlockList                 []uint32
	IPBlockList                   []string
	IPBlockListURL                []string
	IPBlockListFile               []string
	IgnoreByDownloaded            uint32
	GenIPDat                      uint32
	IPUploadedCheck               bool
	IPUpCheckInterval             uint32
	IPUpCheckIncrementMB          uint32
	IPUpCheckPerTorrentRatio      float64 `json:"ipUpCheckPerTorrentRatio" toml:"ipUpCheckPerTorrentRatio"`
	MaxIPPortCount                uint32  `json:"maxIPPortCount" toml:"maxIPPortCount"`
	BanByProgressUploaded         bool    `json:"banByProgressUploaded" toml:"banByProgressUploaded"`
	BanByPUStartMB                uint32  `json:"banByPUStartMB" toml:"banByPUStartMB"`
	BanByPUStartPercent           float64 `json:"banByPUStartPercent" toml:"banByPUStartPercent"`
	BanByPUAntiErrorRatio         float64 `json:"banByPUAntiErrorRatio" toml:"banByPUAntiErrorRatio"`
	BanByRelativeProgressUploaded bool    `json:"banByRelativeProgressUploaded" toml:"banByRelativeProgressUploaded"`
	BanByRelativePUStartMB        uint32  `json:"banByRelativePUStartMB" toml:"banByRelativePUStartMB"`
	BanByRelativePUStartPercent   float64 `json:"banByRelativePUStartPercent" toml:"banByRelativePUStartPercent"`
	BanByRelativePUAntiErrorRatio float64 `json:"banByRelativePUAntiErrorRatio" toml:"banByRelativePUAntiErrorRatio"`
}

var programName = "qBittorrent-ClientBlocker"
var programVersion = "Unknown"
var programUserAgent = fmt.Sprintf("%s/%s (%s, %s)", programName, programVersion, runtime.GOOS, runtime.GOARCH)

var shortFlag_ShowVersion bool
var longFlag_ShowVersion bool
var startDelay uint
var noChdir bool
var needRegHotKey bool
var needHideWindow bool
var needHideSystray bool

var randomStrRegexp = regexp2.MustCompile("[a-zA-Z0-9]{32}", 0)
var blockListCompiled sync.Map
var ipBlockListCompiled sync.Map
var blockListURLLastFetch int64 = 0
var ipBlockListURLLastFetch int64 = 0

// blockListFileLastMod 记录黑名单文件的最后修改时间, 用于热重载判断.
var blockListFileLastMod = make(map[string]int64)

// ipBlockListFileLastMod 记录 IP 黑名单文件的最后修改时间.
var ipBlockListFileLastMod = make(map[string]int64)

// lastModMutex 用于保护上述 LastMod Map 的并发读写.
var lastModMutex sync.RWMutex

// currentTimestamp 记录当前的 UNIX 时间戳.
var currentTimestamp int64 = 0
var programStartTimestamp = time.Now().Unix()

// configLastFetch 记录上次加载配置的具体时间.
var configLastFetch int64 = 0
var cookieJar, _ = cookiejar.New(nil)

var lastURL = ""
var configLastMod = make(map[string]int64)
var configFilename string = "config.json"
var shortFlag_configFilename string
var longFlag_configFilename string
var additionConfigFilename string = "config_additional.json"
var shortFlag_additionConfigFilename string
var longFlag_additionConfigFilename string

var httpTransport = &http.Transport{
	DisableKeepAlives:     true,
	ForceAttemptHTTP2:     false,
	MaxConnsPerHost:       32,
	MaxIdleConns:          32,
	MaxIdleConnsPerHost:   32,
	IdleConnTimeout:       60 * time.Second,
	TLSHandshakeTimeout:   12 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second,
	TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
	Proxy:                 GetProxy,
}

var httpClient http.Client
var httpClientExternal http.Client // 没有 Cookie.

var configLock sync.RWMutex
var config *ConfigStruct = &ConfigStruct{
	CheckUpdate:                   true,
	Debug:                         false,
	Debug_CheckTorrent:            false,
	Debug_CheckPeer:               false,
	Interval:                      6,
	UpdateInterval:                86400,
	RestartInterval:               6,
	TorrentMapCleanInterval:       60,
	BanTime:                       86400,
	BanAllPort:                    false,
	BanIPCIDR:                     "/32",
	BanIP6CIDR:                    "/128",
	IgnoreEmptyPeer:               true,
	IgnoreNoLeechersTorrent:       false,
	IgnorePTTorrent:               true,
	IgnoreFailureExit:             false,
	SleepTime:                     20,
	Timeout:                       6,
	Proxy:                         "Auto",
	LongConnection:                true,
	LogPath:                       "logs",
	LogToFile:                     true,
	LogDebug:                      false,
	Listen:                        "127.0.0.1:26262",
	WebUI:                         false,
	WebUIListen:                   "127.0.0.1:7222",
	WebUIUsername:                 "",
	WebUIPassword:                 "",
	ClientType:                    "",
	ClientURL:                     "",
	ClientUsername:                "",
	ClientPassword:                "",
	UseBasicAuth:                  false,
	UseShadowBan:                  true,
	SkipCertVerification:          false,
	FetchFailedThreshold:          0,
	ExecCommand_FetchFailed:       "",
	ExecCommand_Run:               "",
	ExecCommand_Ban:               "",
	ExecCommand_Unban:             "",
	SyncServerURL:                 "",
	SyncServerToken:               "",
	BTNConfigureURL:               "",
	BTNAppID:                      "",
	BTNAppSecret:                  "",
	BTNSyncRules:                  true,
	BTNSyncException:              true,
	BTNSubmitPeers:                false,
	BTNSubmitBans:                 false,
	BTNSubmitHistories:            false,
	BlockList:                     []string{},
	BlockListURL:                  []string{},
	BlockListFile:                 []string{},
	PortBlockList:                 []uint32{},
	IPBlockList:                   []string{},
	IPBlockListURL:                []string{},
	IPBlockListFile:               []string{},
	IgnoreByDownloaded:            100,
	GenIPDat:                      0,
	IPUploadedCheck:               false,
	IPUpCheckInterval:             300,
	IPUpCheckIncrementMB:          38000,
	IPUpCheckPerTorrentRatio:      3,
	MaxIPPortCount:                0,
	BanByProgressUploaded:         false,
	BanByPUStartMB:                20,
	BanByPUStartPercent:           2,
	BanByPUAntiErrorRatio:         3,
	BanByRelativeProgressUploaded: false,
	BanByRelativePUStartMB:        20,
	BanByRelativePUStartPercent:   3,
	BanByRelativePUAntiErrorRatio: 3,
}

var httpServer = http.Server{
	ReadTimeout:  30 * time.Second,
	WriteTimeout: 30 * time.Second,
	Handler:      &httpServerHandler{},
}

func SetBlockListFromContent(blockListContent []string, blockListSource string) int {
	setCount := 0

	for index, content := range blockListContent {
		content = StrTrim(ProcessRemark(content))
		if content == "" {
			Log("Debug-SetBlockListFromContent_Compile", GetLangText("Error-Debug-EmptyLineWithSource"), false, index, blockListSource)
			continue
		}

		if _, exists := blockListCompiled.Load(content); exists {
			continue
		}

		Log("Debug-SetBlockListFromContent_Compile", ":%d %s (Source: %s)", false, index, content, blockListSource)

		reg, err := regexp2.Compile("(?i)"+content, 0)
		if err != nil {
			Log("SetBlockListFromContent_Compile", GetLangText("Error-SetBlockListFromContent_Compile"), true, index, content, blockListSource)
			continue
		}

		reg.MatchTimeout = 50 * time.Millisecond

		blockListCompiled.Store(content, reg)
		setCount++
	}

	return setCount
}
func SetBlockListFromFile() bool {
	if len(config.BlockListFile) == 0 {
		return true
	}

	setCount := 0
	updated := false

	for _, filePath := range config.BlockListFile {
		blockListFileStat, err := os.Stat(filePath)
		if err != nil {
			Log("SetBlockListFromFile", GetLangText("Error-LoadFile"), false, filePath, err.Error())
			return false
		}

		// 最大 8MB.
		if blockListFileStat.Size() > 8388608 {
			Log("SetBlockListFromFile", GetLangText("Error-LargeFile"), true)
			continue
		}

		// 获取当前文件的最后修改时间.
		fileLastMod := blockListFileStat.ModTime().Unix()
		// 为了线程安全, 先加读锁获取 Map 中的旧值到局部变量 lastMod.
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
			Log("SetBlockListFromFile", GetLangText("Error-LoadFile"), true, filePath, err.Error())
			return false
		}

		// 处理完成后, 加写锁更新 Map.
		lastModMutex.Lock()
		blockListFileLastMod[filePath] = fileLastMod
		lastModMutex.Unlock()

		var content []string
		if filepath.Ext(filePath) == ".json" {
			err = json.Unmarshal(jsonc.ToJSON(blockListContent), &content)
			if err != nil {
				Log("SetBlockListFromFile", GetLangText("Error-GenJSONWithID"), true, filePath, err.Error())
				continue
			}
		} else {
			content = strings.Split(string(blockListContent), "\n")
		}

		setCount += SetBlockListFromContent(content, filePath)
		updated = true
	}

	if updated {
		Log("SetBlockListFromFile", GetLangText("Success-SetBlockListFromFile"), true, setCount)
	}
	return true
}
func SetBlockListFromURL() bool {
	if len(config.BlockListURL) == 0 || (blockListURLLastFetch+int64(config.UpdateInterval)) > currentTimestamp {
		return true
	}

	blockListURLLastFetch = currentTimestamp
	setCount := 0

	for _, blockListURL := range config.BlockListURL {
		httpStatusCode, httpHeader, blockListContent := Fetch(blockListURL, false, false, true, nil)
		if httpStatusCode == 304 {
			continue
		}

		if blockListContent == nil {
			//blockListURLLastFetch -= (int64(config.UpdateInterval) + 900)
			Log("SetBlockListFromURL", GetLangText("Error-FetchResponse2"), true)
			continue
		}

		// 最大 8MB.
		if len(blockListContent) > 8388608 {
			Log("SetBlockListFromURL", GetLangText("Error-LargeFile"), true)
			continue
		}

		var content []string
		if strings.HasSuffix(strings.ToLower(strings.Split(httpHeader.Get("Content-Type"), ";")[0]), "json") {
			err := json.Unmarshal(jsonc.ToJSON(blockListContent), &content)
			if err != nil {
				Log("SetBlockListFromURL", GetLangText("Error-GenJSONWithID"), true, blockListURL, err.Error())
				continue
			}
		} else {
			content = strings.Split(string(blockListContent), "\n")
		}

		setCount += SetBlockListFromContent(content, blockListURL)
	}

	Log("SetBlockListFromURL", GetLangText("Success-SetBlockListFromURL"), true, setCount)
	return true
}
func SetIPBlockListFromContent(ipBlockListContent []string, ipBlockListSource string) int {
	setCount := 0

	for index, content := range ipBlockListContent {
		content = StrTrim(ProcessRemark(content))
		if content == "" {
			Log("Debug-SetIPBlockListFromContent_Compile", GetLangText("Error-Debug-EmptyLineWithSource"), false, index, ipBlockListSource)
			continue
		}

		if _, exists := ipBlockListCompiled.Load(content); exists {
			continue
		}

		Log("Debug-SetIPBlockListFromContent_Compile", ":%d %s (Source: %s)", false, index, content, ipBlockListSource)
		cidr := ParseIPCIDR(content)
		if cidr == nil {
			Log("SetIPBlockListFromContent_Compile", GetLangText("Error-SetIPBlockListFromContent_Compile"), true, index, content, ipBlockListSource)
			continue
		}

		ipBlockListCompiled.Store(content, cidr)
		setCount++
	}

	return setCount
}
func SetIPBlockListFromFile() bool {
	if len(config.IPBlockListFile) == 0 {
		return true
	}

	setCount := 0
	updated := false

	for _, filePath := range config.IPBlockListFile {
		ipBlockListFileStat, err := os.Stat(filePath)
		if err != nil {
			Log("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), false, filePath, err.Error())
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
			Log("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), true, filePath, err.Error())
			return false
		}

		// 加写锁更新.
		lastModMutex.Lock()
		ipBlockListFileLastMod[filePath] = fileLastMod
		lastModMutex.Unlock()

		var content []string
		if filepath.Ext(filePath) == ".json" {
			err := json.Unmarshal(jsonc.ToJSON(ipBlockListFile), &content)
			if err != nil {
				Log("SetIPBlockListFromFile", GetLangText("Error-GenJSONWithID"), true, filePath, err.Error())
			}
		} else {
			content = strings.Split(string(ipBlockListFile), "\n")
		}

		setCount += SetIPBlockListFromContent(content, filePath)
		updated = true
	}

	if updated {
		Log("SetIPBlockListFromFile", GetLangText("Success-SetIPBlockListFromFile"), true, setCount)
	}
	return true
}
func SetIPBlockListFromURL() bool {
	if len(config.IPBlockListURL) == 0 || (ipBlockListURLLastFetch+int64(config.UpdateInterval)) > currentTimestamp {
		return true
	}

	ipBlockListURLLastFetch = currentTimestamp
	setCount := 0

	for _, ipBlockListURL := range config.IPBlockListURL {
		httpStatusCode, httpHeader, ipBlockListContent := Fetch(ipBlockListURL, false, false, true, nil)
		if httpStatusCode == 304 {
			continue
		}

		if ipBlockListContent == nil {
			//ipBlockListURLLastFetch -= (int64(config.UpdateInterval) + 900)
			Log("SetIPBlockListFromURL", GetLangText("Error-FetchResponse2"), true)
			continue
		}

		if len(ipBlockListContent) > 8388608 {
			Log("SetIPBlockListFromURL", GetLangText("Error-LargeFile"), true)
			continue
		}

		var content []string
		if strings.HasSuffix(httpHeader.Get("Content-Type"), "json") {
			err := json.Unmarshal(jsonc.ToJSON(ipBlockListContent), &content)
			if err != nil {
				Log("SetIPBlockListFromURL", GetLangText("Error-GenJSONWithID"), true, ipBlockListURL, err.Error())
				continue
			}
		} else {
			content = strings.Split(string(ipBlockListContent), "\n")
		}

		setCount += SetIPBlockListFromContent(content, ipBlockListURL)
	}

	Log("SetIPBlockListFromURL", GetLangText("Success-SetIPBlockListFromURL"), true, setCount)

	return true
}
func LoadConfig(filename string, notExistErr bool, targetConfig *ConfigStruct) int {
	configFileStat, err := os.Stat(filename)
	if err != nil {
		notExist := os.IsNotExist(err)
		if notExistErr || !notExist {
			Log("Debug-LoadConfig", GetLangText("Error-LoadConfigMeta"), false, filename, err.Error())
		}
		if notExist {
			return -5
		}

		return -2
	}

	tmpConfigLastMod := configFileStat.ModTime().Unix()
	// 加读锁获取旧值.
	lastModMutex.RLock()
	lastMod := configLastMod[filename]
	lastModMutex.RUnlock()

	if tmpConfigLastMod <= lastMod {
		return -1
	}

	if lastMod != 0 {
		Log("Debug-LoadConfig", GetLangText("Debug-LoadConfig_HotReload"), false, filename)
	}

	configFile, err := os.ReadFile(filename)
	if err != nil {
		Log("LoadConfig", GetLangText("Error-LoadConfig"), true, filename, err.Error())
		return -3
	}

	// 加写锁更新.
	lastModMutex.Lock()
	configLastMod[filename] = tmpConfigLastMod
	lastModMutex.Unlock()

	switch filepath.Ext(strings.ToLower(filename)) {
	case ".json":
		if err := json.Unmarshal(jsonc.ToJSON(configFile), targetConfig); err != nil {
			Log("LoadConfig", GetLangText("Error-ParseConfig"), true, filename, err.Error())
			return -4
		}
	case ".toml":
		if err := toml.Unmarshal(configFile, targetConfig); err != nil {
			Log("LoadConfig", GetLangText("Error-ParseConfig"), true, filename, err.Error())
			return -4
		}
	}

	Log("LoadConfig", GetLangText("Success-LoadConfig"), true, filename)

	return 0
}
func InitConfig() {
	if config.Interval < 1 {
		config.Interval = 1
	}

	if config.Timeout < 1 {
		config.Timeout = 1
	}

	if config.ClientURL != "" {
		config.ClientURL = strings.TrimRight(config.ClientURL, "/")
	}

	if config.SkipCertVerification {
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} else {
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: false}
	}

	httpTransportExternal := httpTransport.Clone()

	if config.Proxy == "Auto" {
		// 默认模式, 仅对外部资源使用代理.
		httpTransport.Proxy = nil
		httpTransportExternal.Proxy = GetProxy
	} else if config.Proxy == "All" {
		httpTransport.Proxy = GetProxy
		httpTransportExternal.Proxy = GetProxy
	} else {
		httpTransport.Proxy = nil
		httpTransportExternal.Proxy = nil
	}

	if config.LongConnection {
		httpTransport.DisableKeepAlives = false
	}

	currentTimeout := time.Duration(config.Timeout) * time.Second

	httpClient = http.Client{
		Timeout:   currentTimeout,
		Jar:       cookieJar,
		Transport: httpTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	httpClientExternal = http.Client{
		Timeout:   currentTimeout,
		Transport: httpTransportExternal,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	httpServer.ReadTimeout = currentTimeout
	httpServer.WriteTimeout = currentTimeout

	t := reflect.TypeOf(*config)
	v := reflect.ValueOf(*config)
	for k := 0; k < t.NumField(); k++ {
		Log("LoadConfig_Current", "%v: %v", false, t.Field(k).Name, FormatConfigValueForLog(t.Field(k).Name, v.Field(k).Interface()))
	}

	EraseSyncMap(&blockListCompiled)
	blockListURLLastFetch = 0
	SetBlockListFromContent(config.BlockList, "BlockList")

	EraseSyncMap(&ipBlockListCompiled)
	ipBlockListURLLastFetch = 0
	SetIPBlockListFromContent(config.IPBlockList, "IPBlockList")
}

func FormatConfigValueForLog(fieldName string, value interface{}) interface{} {
	fieldName = strings.ToLower(fieldName)

	if strings.Contains(fieldName, "password") || strings.Contains(fieldName, "secret") || strings.HasSuffix(fieldName, "token") {
		switch typedValue := value.(type) {
		case string:
			if typedValue == "" {
				return ""
			}
		case []string:
			if len(typedValue) == 0 {
				return typedValue
			}
		}

		return "[REDACTED]"
	}

	return value
}
func LoadInitConfig(firstLoad bool) bool {
	newConfig := *config
	loadConfigStatus := LoadConfig(configFilename, true, &newConfig)

	if loadConfigStatus < -1 {
		Log("LoadInitConfig", GetLangText("Failed-LoadInitConfig"), true)
	} else {
		loadAdditionalConfigStatus := LoadConfig(additionConfigFilename, false, &newConfig)
		if loadAdditionalConfigStatus == -5 && additionConfigFilename == "config_additional.json" {
			loadAdditionalConfigStatus = LoadConfig("config/"+additionConfigFilename, false, &newConfig)
		}

		if loadConfigStatus == 0 || loadAdditionalConfigStatus == 0 {
			configLock.Lock()
			config = &newConfig
			configLock.Unlock()
			InitConfig()
		}
	}

	if !LoadLog() {
		CloseLogFile()
	}

	if firstLoad {
		GetProxy(nil)
		SetURLFromClient()
	}

	if config.ClientURL != "" {
		if lastURL != config.ClientURL {
			if !DetectClient() {
				Log("LoadInitConfig", GetLangText("LoadInitConfig_DetectClientFailed"), true)
				return false
			}
			if !Login() {
				Log("LoadInitConfig", GetLangText("LoadInitConfig_AuthFailed"), true)
				return false
			}
			InitClient()
			SubmitBlockPeer(nil)
			lastURL = config.ClientURL
		}
	} else {
		// 重置为上次使用的 URL, 主要目的是防止热重载配置文件破坏首次启动后从 qBittorrent 配置文件读取的 URL.
		config.ClientURL = lastURL
	}

	if config.UseShadowBan && TestShadowBanAPI() <= 0 {
		config.UseShadowBan = false
	}

	if !firstLoad {
		SetBlockListFromFile()
		SetIPBlockListFromFile()
		GoWithCrashLog("SetBlockListFromURL", func() {
			SetBlockListFromURL()
		})
		GoWithCrashLog("SetIPBlockListFromURL", func() {
			SetIPBlockListFromURL()
		})
		GoWithCrashLog("BTN_GetConfig", BTN_GetConfig)
	}

	return true
}
func RegFlag() {
	flag.BoolVar(&shortFlag_ShowVersion, "v", false, GetLangText("ProgramVersion"))
	flag.BoolVar(&longFlag_ShowVersion, "version", false, GetLangText("ProgramVersion"))
	flag.StringVar(&shortFlag_configFilename, "c", "", GetLangText("ConfigPath"))
	flag.StringVar(&longFlag_configFilename, "config", "", GetLangText("ConfigPath"))
	flag.StringVar(&shortFlag_additionConfigFilename, "ca", "", GetLangText("AdditionalConfigPath"))
	flag.StringVar(&longFlag_additionConfigFilename, "config_additional", "", GetLangText("AdditionalConfigPath"))
	flag.BoolVar(&config.Debug, "debug", false, GetLangText("DebugMode"))
	flag.UintVar(&startDelay, "startdelay", 0, GetLangText("StartDelay"))
	flag.BoolVar(&noChdir, "nochdir", false, GetLangText("NoChdir"))
	flag.BoolVar(&needRegHotKey, "reghotkey", true, GetLangText("RegHotKey"))
	flag.BoolVar(&needHideWindow, "hidewindow", false, GetLangText("HideWindow"))
	flag.BoolVar(&needHideSystray, "hidesystray", false, GetLangText("HideSystray"))
	flag.Parse()
}
func ShowVersion() {
	Log("ShowVersion", "%s %s", false, programName, programVersion)
}
func PrepareEnv() bool {
	LoadLang(GetLangCode())
	RegFlag()
	ShowVersion()
	log.SetFlags(0)
	log.SetOutput(logwriter)

	if shortFlag_ShowVersion || longFlag_ShowVersion {
		return false
	}

	if longFlag_configFilename != "" {
		configFilename = longFlag_configFilename
	} else if shortFlag_configFilename != "" {
		configFilename = shortFlag_configFilename
	}

	if longFlag_additionConfigFilename != "" {
		additionConfigFilename = longFlag_additionConfigFilename
	} else if shortFlag_additionConfigFilename != "" {
		additionConfigFilename = shortFlag_additionConfigFilename
	}

	path, err := os.Executable()
	if err != nil {
		Log("PrepareEnv", GetLangText("Error-DetectProgramPath"), false, err.Error())
		return false
	}

	if !noChdir {
		programDir := filepath.Dir(path)
		if os.Chdir(programDir) == nil {
			Log("PrepareEnv", GetLangText("Success-ChangeWorkingDir"), false, programDir)
			LoadLang(GetLangCode())
		} else {
			Log("PrepareEnv", GetLangText("Failed-ChangeWorkingDir"), false, programDir)
		}
	}

	return true
}
