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

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/dlclark/regexp2"
	"github.com/tidwall/jsonc"
)

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
var BlockListCompiled = mapset.NewSet[*regexp2.Regexp]()
var ipBlockListCompiled = make(map[string]*net.IPNet)

var cookieJar, _ = cookiejar.New(nil)

var lastURL = ""
var configLastMod = make(map[string]int64)
var configFilename string = "config.json"
var shortFlag_configFilename string
var longFlag_configFilename string
var additionConfigFilename string = "config_additional.json"
var shortFlag_additionConfigFilename string
var longFlag_additionConfigFilename string

// 这几个鬼玩意可以考虑改成map
var blockListURLLastModTime []int64 = make([]int64, len(config.BlockListURL))
var blockListURLEtag []string = make([]string, len(config.BlockListURL))
var blockListFileLastMod []int64 = make([]int64, len(config.BlockListFile))
var blockListURLLastFetchTime int64 = 0
var ipBlockListURLLastModTime []int64 = make([]int64, len(config.IPBlockListURL))
var ipBlockListURLEtag []string = make([]string, len(config.IPBlockListURL))
var ipBlockListFileLastMod []int64 = make([]int64, len(config.IPBlockListFile))
var ipblockListURLLastFetchTime int64 = 0

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
var HttpServer = http.Server{
	ReadTimeout:  30,
	WriteTimeout: 30,
	Handler:      &HttpServerHandler{},
}

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
	BanByPUStartPrecent:           2,
	BanByPUAntiErrorRatio:         3,
	BanByRelativeProgressUploaded: false,
	BanByRelativePUStartMB:        20,
	BanByRelativePUStartPrecent:   2,
	BanByRelativePUAntiErrorRatio: 3,
}

func SetBlockListFromContent(blockListContent []string) int {
	if blockListContent == nil { // 有可能不接入内容
		return 0
	}
	prev_count := BlockListCompiled.Cardinality()
	for index, item := range blockListContent {
		reg, err := regexp2.Compile("(?i)"+item, 0)
		if err != nil {
			Log("Error-SetBlocklistFromContent_Compile", GetLangText("Error-SetBlockList_Compile"), true, index, item, err.Error())
			continue
		}
		reg.MatchTimeout = 50 * time.Millisecond
		BlockListCompiled.Add(reg)
	}

	return BlockListCompiled.Cardinality() - prev_count
}
func SetBlockListFromFile() {
	if config.BlockListFile == nil || len(config.BlockListFile) == 0 {
		return
	}
	for index, filepath := range config.BlockListFile {
		blockListFileStat, err := os.Stat(filepath)
		if err != nil {
			Log("SetBlockListFromFile", GetLangText("Error-LoadFile"), true, filepath, err.Error())
		}
		if blockListFileStat.ModTime().Unix() == blockListFileLastMod[index] {
			continue
		}
		blockListFileLastMod[index] = blockListFileStat.ModTime().Unix()
		blockListFileRaw, _ := os.ReadFile(filepath)
		var blockListContent []string
		if filepath[len(filepath)-5:] == ".json" {
			err = json.Unmarshal(blockListFileRaw, &blockListContent)
			if err != nil {
				Log("SetBlockListFromFile", GetLangText("Error-GenJSON"), true, filepath, err.Error())
				return
			}
		} else {
			blockListContent = strings.Split(string(blockListFileRaw), "\n")
			for index, blockListLine := range blockListContent {
				blockListContent[index] = StrTrim(strings.SplitN(blockListLine, "#", 2)[0])
			}
		}
		Log("SetBlockListFromFile", GetLangText("Success-SetBlockListFromFile"), true, SetBlockListFromContent(blockListContent))
	}
}
func SetBlockListFromURL() {
	if config.BlockListURL == nil || len(config.BlockListURL) == 0 || (blockListURLLastFetchTime+int64(config.UpdateInterval)) > CurrentTimestamp {
		return
	}
	blockListURLLastFetchTime = CurrentTimestamp
	for _, blockListURLItem := range config.BlockListURL {
		// 这里可以拉上并发,url没必要这样等
		// 使用 Etag 和 Date 检测 URL 内容是否刷新
		_, HttpHeader, blockListRaw := Fetch(blockListURLItem, false, false, nil)
		StatusCode := HttpHeader.Get("status")
		if StatusCode != "200" || blockListRaw == nil {
			Log("SetBlockListFromURL", GetLangText("Error-FetchResponse2"), true, blockListURLItem)
			continue
		}
		var blockListContent []string
		if HttpHeader.Get("Content-Type")[len(HttpHeader["Content-Type"][0])-4:] == "json" {
			err := json.Unmarshal(blockListRaw, &blockListContent)
			if err != nil {
				Log("SetBlockListFromURL", GetLangText("Error-Unmarshal"), true)
				return
			}
		} else {
			blockListContent = strings.Split(string(blockListRaw), "\n")
			for index, blockListLine := range blockListContent {
				blockListContent[index] = StrTrim(strings.SplitN(blockListLine, "#", 2)[0])
			}
		}
		Log("SetBlockListFromURL", GetLangText("Success-SetBlockListFromURL"), true, blockListURLItem, SetBlockListFromContent(blockListContent))
	}
}

func SetIPBlockListFromContent(ipBlockListContent []byte) int {
	// Max 8MB.
	if len(ipBlockListContent) > 8388608 {
		Log("SetIPBlockListFromContent", GetLangText("Error-LargeFile"), true)
		return 0
	}
	prev_count := len(ipBlockListCompiled)
	ipBlockListArr := strings.Split(string(ipBlockListContent), "\n")

	for ipBlockListLineNum, ipBlockListLine := range ipBlockListArr {

		ipBlockListLine = StrTrim(strings.SplitN(ipBlockListLine, "#", 2)[0])
		if ipBlockListLine == "" {
			Log("Debug-SetIPBlockList_Compile", GetLangText("Error-Debug-EmptyLine"), false, ipBlockListLineNum+1)
			continue
		}
		if ipBlockListCompiled[ipBlockListLine] != nil {
			Log("Debug-SetIPBlockList_Duplicates", ":%d %s", false, ipBlockListLineNum+1, ipBlockListLine)
			continue
		}
		Log("Debug-SetIPBlockList_Compile", ":%d %s", false, ipBlockListLineNum+1, ipBlockListLine)
		cidr := ParseIPCIDR(ipBlockListLine)
		if cidr == nil {
			Log("SetIPBlockList_Compile", GetLangText("Error-SetIPBlockList_Compile"), true, ipBlockListLineNum+1, ipBlockListLine)
			continue
		}

		ipBlockListCompiled[ipBlockListLine] = cidr
	}

	return len(ipBlockListCompiled) - prev_count
}
func SetIPBlockListFromFile() bool {
	if config.IPBlockListFile == nil || len(config.IPBlockListFile) == 0 {
		return true
	}
	// todo , need support array
	for index, filepath := range config.IPBlockListFile {
		ipBlockListFileStat, err := os.Stat(filepath)
		if err != nil {
			Log("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), false, filepath, err.Error())
			continue
		}
		if ipBlockListFileLastMod[index] == ipBlockListFileStat.ModTime().Unix() {
			continue
		}
		ipBlockListFileLastMod[index] = ipBlockListFileStat.ModTime().Unix()
		ipBlockListFileRaw, err := os.ReadFile(filepath)
		if err != nil {
			Log("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), true, config.IPBlockListFile, err.Error())
			continue
		}
		AddCount := SetIPBlockListFromContent(ipBlockListFileRaw)
		Log("SetIPBlockListFromFile", GetLangText("Success-SetIPBlockListFromFile"), true, filepath, AddCount)
	}
	return true
}
func SetIPBlockListFromURL() bool {
	if config.IPBlockListURL == nil || len(config.IPBlockListURL) == 0 || (ipblockListURLLastFetchTime+int64(config.UpdateInterval)) > CurrentTimestamp {
		return true
	}

	ipblockListURLLastFetchTime = CurrentTimestamp
	// todo, need support array
	// use Etag, Date test url last fetch time
	// ipBlockListURLLastFetch = CurrentTimestamp
	for _, IPBlockListURLItem := range config.IPBlockListURL {
		_, HttpHeader, ipBlockListRaw := Fetch(IPBlockListURLItem, false, false, nil)
		if HttpHeader.Get("status") != "200" || ipBlockListRaw == nil {
			Log("SetIPBlockListFromURL", GetLangText("Error-FetchResponse2"), true, IPBlockListURLItem)
			continue
		}
		AddCount := SetIPBlockListFromContent(ipBlockListRaw)
		Log("SetIPBlockListFromURL", GetLangText("Success-SetIPBlockListFromURL"), true, IPBlockListURLItem, AddCount)
	}
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

	blockListURLLastModTime = make([]int64, len(config.BlockListURL))
	blockListURLEtag = make([]string, len(config.BlockListURL))
	blockListFileLastMod = make([]int64, len(config.BlockListFile))
	blockListURLLastFetchTime = 0
	ipBlockListURLLastModTime = make([]int64, len(config.IPBlockListURL))
	ipBlockListURLEtag = make([]string, len(config.IPBlockListURL))
	ipBlockListFileLastMod = make([]int64, len(config.IPBlockListFile))
	ipblockListURLLastFetchTime = 0

	SetBlockListFromContent(config.BlockList)
	SetIPBlockListFromContent([]byte(strings.Join(config.IPBlockList, "\n")))
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

	HttpServer.ReadTimeout = currentTimeout
	HttpServer.WriteTimeout = currentTimeout

	t := reflect.TypeOf(config)
	v := reflect.ValueOf(config)
	for k := 0; k < t.NumField(); k++ {
		Log("LoadConfig_Current", "%v: %v", false, t.Field(k).Name, v.Field(k).Interface())
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

	if !LoadLog() && LogFile != nil {
		LogFile.Close()
		LogFile = nil
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
		// 现在加载内部的配置可方便了,但是还是不要用这个的好

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
