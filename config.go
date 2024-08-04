package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
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
	SleepTime                     uint32
	Timeout                       uint32
	Proxy                         string
	LongConnection                bool
	LogPath                       string
	LogToFile                     bool
	LogDebug                      bool
	Listen                        string
	ClientType                    string
	ClientURL                     string
	ClientUsername                string
	ClientPassword                string
	UseBasicAuth                  bool
	SkipCertVerification          bool
	FetchFailedThreshold          int
	ExecCommand_FetchFailed       string
	ExecCommand_Run               string
	ExecCommand_Ban               string
	ExecCommand_Unban             string
	SyncServerURL                 string
	SyncServerToken               string
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
	IPUpCheckPerTorrentRatio      float64
	MaxIPPortCount                uint32
	BanByProgressUploaded         bool
	BanByPUStartMB                uint32
	BanByPUStartPrecent           float64
	BanByPUAntiErrorRatio         float64
	BanByRelativeProgressUploaded bool
	BanByRelativePUStartMB        uint32
	BanByRelativePUStartPrecent   float64
	BanByRelativePUAntiErrorRatio float64
}

var programName = "qBittorrent-ClientBlocker"
var programVersion = "Unknown"
var programUserAgent = programName + "/" + programVersion

var shortFlag_ShowVersion bool
var longFlag_ShowVersion bool
var startDelay uint
var noChdir bool
var needHideWindow bool
var needHideSystray bool

var randomStrRegexp = regexp2.MustCompile("[a-zA-Z0-9]{32}", 0)
var blockListCompiled = make(map[string]*regexp2.Regexp)
var ipBlockListCompiled = make(map[string]*net.IPNet)
var blockListURLLastFetch int64 = 0
var ipBlockListURLLastFetch int64 = 0
var blockListFileLastMod = make(map[string]int64)
var ipBlockListFileLastMod = make(map[string]int64)
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
var httpClientWithoutCookie http.Client

var httpServer = http.Server{
	ReadTimeout:  30,
	WriteTimeout: 30,
	Handler:      &httpServerHandler{},
}

var config = ConfigStruct{
	CheckUpdate:                   true,
	Debug:                         false,
	Debug_CheckTorrent:            false,
	Debug_CheckPeer:               false,
	Interval:                      6,
	CleanInterval:                 3600,
	UpdateInterval:                86400,
	RestartInterval:               6,
	TorrentMapCleanInterval:       60,
	BanTime:                       86400,
	BanAllPort:                    false,
	BanIPCIDR:                     "/32",
	BanIP6CIDR:                    "/128",
	IgnoreEmptyPeer:               true,
	IgnoreNoLeechersTorrent:       true,
	IgnorePTTorrent:               true,
	SleepTime:                     20,
	Timeout:                       6,
	Proxy:                         "Auto",
	LongConnection:                true,
	LogPath:                       "logs",
	LogToFile:                     true,
	LogDebug:                      false,
	Listen:                        "127.0.0.1:26262",
	ClientType:                    "",
	ClientURL:                     "",
	ClientUsername:                "",
	ClientPassword:                "",
	UseBasicAuth:                  false,
	SkipCertVerification:          false,
	FetchFailedThreshold:          0,
	ExecCommand_FetchFailed:       "",
	ExecCommand_Run:               "",
	ExecCommand_Ban:               "",
	ExecCommand_Unban:             "",
	SyncServerURL:                 "",
	SyncServerToken:               "",
	BlockList:                     []string{},
	BlockListURL:                  []string{},
	BlockListFile:                 []string{},
	PortBlockList:                 []uint32{},
	IPBlockList:                   []string{},
	IPBlockListURL:                nil,
	IPBlockListFile:               nil,
	IgnoreByDownloaded:            100,
	GenIPDat:                      0,
	IPUploadedCheck:               false,
	IPUpCheckInterval:             300,
	IPUpCheckIncrementMB:          38000,
	IPUpCheckPerTorrentRatio:      3,
	MaxIPPortCount:                0,
	BanByProgressUploaded:         false,
	BanByPUStartMB:                20,
	BanByPUStartPrecent:           2,
	BanByPUAntiErrorRatio:         3,
	BanByRelativeProgressUploaded: false,
	BanByRelativePUStartMB:        20,
	BanByRelativePUStartPrecent:   2,
	BanByRelativePUAntiErrorRatio: 3,
}

func SetBlockListFromContent(blockListContent []string) int {
	setCount := 0

	for index, content := range blockListContent {
		if content == "" {
			Log("Debug-SetBlockListFromContent_Compile", GetLangText("Error-Debug-EmptyLine"), false, index)
			continue
		}

		if _, exists := blockListCompiled[content]; exists {
			continue
		}

		Log("Debug-SetBlockListFromContent_Compile", ":%d %s", false, index, content)

		reg, err := regexp2.Compile("(?i)"+content, 0)
		if err != nil {
			Log("SetBlockListFromContent_Compile", GetLangText("Error-SetBlockListFromContent_Compile"), true, index, content)
			continue
		}

		reg.MatchTimeout = 50 * time.Millisecond

		blockListCompiled[content] = reg
		setCount++
	}

	return setCount
}
func SetBlockListFromFile() bool {
	if config.BlockListFile == nil || len(config.BlockListFile) == 0 {
		return true
	}
	setCount := 0
	for _, FilePath := range config.BlockListFile {
		blockListFileStat, err := os.Stat(FilePath)
		// Max 8MB.
		if blockListFileStat.Size() > 8388608 {
			Log("SetBlockListFromFile", GetLangText("Error-LargeFile"), true)
			continue
		}
		if err != nil {
			Log("SetBlockListFromFile", GetLangText("Error-LoadFile"), false, FilePath, err.Error())
			return false
		}

		FileLastMod := blockListFileStat.ModTime().Unix()
		if FileLastMod == blockListFileLastMod[FilePath] {
			return false
		}

		var Content []string

		if blockListFileLastMod[FilePath] != 0 {
			Log("Debug-SetBlockListFromFile", GetLangText("Debug-SetBlockListFromFile_HotReload"), false, FilePath)
		}

		blockListContent, err := os.ReadFile(FilePath)
		if err != nil {
			Log("SetBlockListFromFile", GetLangText("Error-LoadFile"), true, FilePath, err.Error())
			return false
		}

		if filepath.Ext(FilePath) == ".json" {
			err = json.Unmarshal(blockListContent, &Content)
			if err != nil {
				Log("SetBlockListFromFile", GetLangText("Error-GenJSON"), true, FilePath)
				continue
			}
		} else {
			Content = strings.Split(string(blockListContent), "\n")
			for index, str := range Content {
				Content[index] = ProcessRemark(str)
			}
		}

		blockListFileLastMod[FilePath] = FileLastMod
		setCount += SetBlockListFromContent(Content)
	}

	Log("SetBlockListFromFile", GetLangText("Success-SetBlockListFromFile"), true, setCount)
	return true
}
func SetBlockListFromURL() bool {
	if config.BlockListURL == nil || len(config.BlockListURL) == 0 || (blockListURLLastFetch+int64(config.UpdateInterval)) > currentTimestamp {
		return true
	}
	blockListURLLastFetch = currentTimestamp
	setCount := 0
	for _, BlockListURL := range config.BlockListURL {
		_, httpHeader, blockListContent := Fetch(BlockListURL, false, false, nil)

		if blockListContent == nil {
			blockListURLLastFetch -= (int64(config.UpdateInterval) + 900)
			Log("SetBlockListFromURL", GetLangText("Error-FetchResponse2"), true)
			return false
		}
		if len(blockListContent) > 8388608 {
			Log("SetBlockListFromURL", GetLangText("Error-LargeFile"), true)
			continue
		}

		var Content []string
		if strings.HasSuffix(httpHeader.Get("Content-Type"), "json") {
			err := json.Unmarshal(blockListContent, &Content)
			if err != nil {
				Log("SetBlockListFromFile", GetLangText("Error-GenJSON"), true, BlockListURL)
			}
		} else {
			Content = strings.Split(string(blockListContent), "\n")
			for index, str := range Content {
				Content[index] = ProcessRemark(str)
			}
		}
		setCount += SetBlockListFromContent(Content)
	}

	Log("SetBlockListFromURL", GetLangText("Success-SetBlockListFromURL"), true, setCount)
	return true
}
func SetIPBlockListFromContent(ipBlockListContent []string) int {
	setCount := 0

	for index, content := range ipBlockListContent {
		if content == "" {
			Log("Debug-SetIPBlockListFromContent_Compile", GetLangText("Error-Debug-EmptyLine"), false, index)
			continue
		}

		if _, exists := ipBlockListCompiled[content]; exists {
			continue
		}

		Log("Debug-SetIPBlockListFromContent_Compile", ":%d %s", false, index, content)
		cidr := ParseIPCIDR(content)
		if cidr == nil {
			Log("SetIPBlockListFromContent_Compile", GetLangText("Error-SetIPBlockListFromContent_Compile"), true, index, content)
			continue
		}

		ipBlockListCompiled[content] = cidr
		setCount++
	}

	return setCount
}
func SetIPBlockListFromFile() bool {
	if config.IPBlockListFile == nil || len(config.IPBlockListFile) == 0 {
		return true
	}
	setCount := 0
	for _, FilePath := range config.IPBlockListFile {
		ipBlockListFileStat, err := os.Stat(FilePath)
		if err != nil {
			Log("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), false, FilePath, err.Error())
			return false
		}

		FileLastMod := ipBlockListFileStat.ModTime().Unix()
		if FileLastMod == ipBlockListFileLastMod[FilePath] {
			return false
		}

		if ipBlockListFileLastMod[FilePath] != 0 {
			Log("Debug-SetIPBlockListFromFile", GetLangText("Debug-SetIPBlockListFromFile_HotReload"), false, FilePath)
		}

		ipBlockListFile, err := os.ReadFile(FilePath)
		if err != nil {
			Log("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), true, FilePath, err.Error())
			return false
		}
		ipBlockListFileLastMod[FilePath] = FileLastMod

		var Content []string
		if filepath.Ext(FilePath) == ".json" {
			err := json.Unmarshal(ipBlockListFile, &Content)
			if err != nil {
				Log("SetIPBlockListFromFile", GetLangText("Error-GenJSON"), true, FilePath)
			}
		} else {
			Content = strings.Split(string(ipBlockListFile), "\n")
			for index, str := range Content {
				Content[index] = ProcessRemark(str)
			}
		}

		setCount += SetIPBlockListFromContent(Content)
	}
	Log("SetIPBlockListFromFile", GetLangText("Success-SetIPBlockListFromFile"), true, setCount)
	return true
}
func SetIPBlockListFromURL() bool {
	if config.IPBlockListURL == nil || len(config.IPBlockListURL) == 0 || (ipBlockListURLLastFetch+int64(config.UpdateInterval)) > currentTimestamp {
		return true
	}
	setCount := 0
	ipBlockListURLLastFetch = currentTimestamp

	for _, ipBlockListURL := range config.IPBlockListURL {
		_, httpHeader, ipBlockListContent := Fetch(ipBlockListURL, false, false, nil)
		if ipBlockListContent == nil {
			ipBlockListURLLastFetch -= (int64(config.UpdateInterval) + 900)
			Log("SetIPBlockListFromURL", GetLangText("Error-FetchResponse2"), true)
			return false
		}
		var Content []string
		if strings.HasSuffix(httpHeader.Get("Content-Type"), "json") {
			err := json.Unmarshal(ipBlockListContent, &Content)
			if err != nil {
				Log("SetIPBlockListFromURL", GetLangText("Error-GenJSON"), true, ipBlockListURL)
			}
		} else {
			Content = strings.Split(string(ipBlockListContent), "\n")
			for index, str := range Content {
				Content[index] = ProcessRemark(str)
			}
		}
		setCount += SetIPBlockListFromContent(Content)
	}

	Log("SetIPBlockListFromURL", GetLangText("Success-SetIPBlockListFromURL"), true, setCount)

	return true
}
func LoadConfig(filename string, notExistErr bool) int {
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
	if tmpConfigLastMod <= configLastMod[filename] {
		return -1
	}

	if configLastMod[filename] != 0 {
		Log("Debug-LoadConfig", GetLangText("Debug-LoadConfig_HotReload"), false, filename)
	}

	configFile, err := os.ReadFile(filename)
	if err != nil {
		Log("LoadConfig", GetLangText("Error-LoadConfig"), true, filename, err.Error())
		return -3
	}

	configLastMod[filename] = tmpConfigLastMod

	if err := json.Unmarshal(jsonc.ToJSON(configFile), &config); err != nil {
		Log("LoadConfig", GetLangText("Error-ParseConfig"), true, filename, err.Error())
		return -4
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

	if config.Proxy == "Auto" {
		httpTransport.Proxy = GetProxy
	} else {
		httpTransport.Proxy = nil
	}

	httpTransportWithoutCookie := httpTransport.Clone()

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

	httpClientWithoutCookie = http.Client{
		Timeout:   currentTimeout,
		Transport: httpTransportWithoutCookie,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	httpServer.ReadTimeout = currentTimeout
	httpServer.WriteTimeout = currentTimeout

	t := reflect.TypeOf(config)
	v := reflect.ValueOf(config)
	for k := 0; k < t.NumField(); k++ {
		Log("LoadConfig_Current", "%v: %v", false, t.Field(k).Name, v.Field(k).Interface())
	}

	blockListCompiled = make(map[string]*regexp2.Regexp)
	SetBlockListFromContent(config.BlockList)
	if errCount := len(config.BlockList) != len(blockListCompiled); errCount {
		Log("LoadConfig_CompileBlockList", GetLangText("Error-CompileBlockList"), false, errCount)
	}

	ipBlockListCompiled = make(map[string]*net.IPNet, len(config.IPBlockList))
	SetIPBlockListFromContent(config.IPBlockList)
	if errCount := len(config.IPBlockList) != len(ipBlockListCompiled); errCount {
		Log("LoadConfig_CompileIPBlockList", GetLangText("Error-CompileIPBlockList"), false, errCount)
	}
}
func LoadInitConfig(firstLoad bool) bool {
	lastURL = config.ClientURL

	loadConfigStatus := LoadConfig(configFilename, true)

	if loadConfigStatus < -1 {
		Log("LoadInitConfig", GetLangText("Failed-LoadInitConfig"), true)
	} else {
		loadAdditionalConfigStatus := LoadConfig(additionConfigFilename, false)
		if loadAdditionalConfigStatus == -5 && additionConfigFilename == "config_additional.json" {
			loadAdditionalConfigStatus = LoadConfig("config/"+additionConfigFilename, false)
		}

		if loadConfigStatus == 0 || loadAdditionalConfigStatus == 0 {
			InitConfig()
		}
	}

	if !LoadLog() && logFile != nil {
		logFile.Close()
		logFile = nil
	}

	if firstLoad {
		GetProxy(nil)
		SetURLFromClient()
	}

	if config.ClientURL != "" {
		if lastURL != config.ClientURL {
			DetectClient()
			InitClient()
			if firstLoad && !Login() {
				return false
			}
			SubmitBlockPeer(nil)
			lastURL = config.ClientURL
		}
	} else {
		// 重置为上次使用的 URL, 主要目的是防止热重载配置文件可能破坏首次启动后从 qBittorrent 配置文件读取的 URL.
		config.ClientURL = lastURL
	}

	if !firstLoad {
		SetBlockListFromFile()
		SetIPBlockListFromFile()
		SetBlockListFromURL()
		SetIPBlockListFromURL()
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
