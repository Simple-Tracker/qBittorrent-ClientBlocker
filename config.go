package main

import (
	"os"
	"net"
	"log"
	"time"
	"flag"
	"reflect"
	"strings"
	"crypto/tls"
	"encoding/json"
	"path/filepath"
	"net/http"
	"net/http/cookiejar"
	"github.com/tidwall/jsonc"
	"github.com/dlclark/regexp2"
)

type ConfigStruct struct {
	CheckUpdate                   bool
	Debug                         bool
	Debug_CheckTorrent            bool
	Debug_CheckPeer               bool
	Interval                      uint32
	CleanInterval                 uint32
	UpdateInterval                uint32
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
	BlockListURL                  string
	BlockListFile                 string
	PortBlockList                 []uint32
	IPBlockList                   []string
	IPBlockListURL                string
	IPBlockListFile               string
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
var blockListCompiled []*regexp2.Regexp
var blockListFromURLCompiled = make(map[string]*regexp2.Regexp)
var blockListFromFileCompiled = make(map[string]*regexp2.Regexp)
var ipBlockListCompiled []*net.IPNet
var ipBlockListFromURLCompiled = make(map[string]*net.IPNet)
var ipBlockListFromFileCompiled = make(map[string]*net.IPNet)
var cookieJar, _ = cookiejar.New(nil)

var lastURL = ""
var configLastMod = make(map[string]int64)
var configFilename string = "config.json"
var shortFlag_configFilename string
var longFlag_configFilename string
var additionConfigFilename string = "config_additional.json"
var shortFlag_additionConfigFilename string
var longFlag_additionConfigFilename string
var blockListURLLastFetch int64 = 0
var ipBlockListURLLastFetch int64 = 0
var blockListFileLastMod int64 = 0
var ipBlockListFileLastMod int64 = 0

var httpTransport = &http.Transport {
	DisableKeepAlives:     true,
	ForceAttemptHTTP2:     false,
	MaxConnsPerHost:       32,
	MaxIdleConns:          32,
	MaxIdleConnsPerHost:   32,
	IdleConnTimeout:       60 * time.Second,
	TLSHandshakeTimeout:   12 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second,
	TLSClientConfig:       &tls.Config { InsecureSkipVerify: false },
	Proxy:                 GetProxy,
}

var httpClient http.Client
var httpClientWithoutCookie http.Client

var httpServer = http.Server {
	ReadTimeout:  30,
	WriteTimeout: 30,
	Handler:      &httpServerHandler {},
}

var config = ConfigStruct {
	CheckUpdate:                   true,
	Debug:                         false,
	Debug_CheckTorrent:            false,
	Debug_CheckPeer:               false,
	Interval:                      6,
	CleanInterval:                 3600,
	UpdateInterval:                86400,
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
	BlockList:                     []string {},
	BlockListURL:                  "",
	BlockListFile:                 "",
	PortBlockList:                 []uint32 {},
	IPBlockList:                   []string {},
	IPBlockListURL:                "",
	IPBlockListFile:               "",
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

func SetBlockListFromContent(blockListContent []byte, blockListCompiled map[string]*regexp2.Regexp) int {
	// Max 8MB.
	if len(blockListContent) > 8388608 {
		Log("SetBlockListFromContent", GetLangText("Error-LargeFile"), true)
		return 0
	}

	blockListArr := strings.Split(string(blockListContent), "\n")
	tmpBlockListCompiled := make(map[string]*regexp2.Regexp)

	for blockListLineNum, blockListLine := range blockListArr {
		blockListLine = StrTrim(strings.SplitN(blockListLine, "#", 2)[0])
		if blockListLine == "" {
			Log("Debug-SetBlockListFromContent_Compile", GetLangText("Error-Debug-EmptyLine"), false, blockListLineNum)
			continue
		}

		if reg, exists := blockListCompiled[blockListLine]; exists {
			tmpBlockListCompiled[blockListLine] = reg
			continue
		}

		Log("Debug-SetBlockListFromContent_Compile", ":%d %s", false, blockListLineNum, blockListLine)

		reg, err := regexp2.Compile("(?i)" + blockListLine, 0)
		if err != nil {
			Log("SetBlockListFromContent_Compile", GetLangText("Error-SetBlockListFromContent_Compile"), true, blockListLineNum, blockListLine)
			continue
		}

		reg.MatchTimeout = 50 * time.Millisecond

		tmpBlockListCompiled[blockListLine] = reg
	}

	blockListCompiled = tmpBlockListCompiled
	return len(blockListCompiled)
}
func SetIPBlockListFromContent(ipBlockListContent []byte, ipBlockListCompiled map[string]*net.IPNet) int {
	// Max 8MB.
	if len(ipBlockListContent) > 8388608 {
		Log("SetIPBlockListFromContent", GetLangText("Error-LargeFile"), true)
		return 0
	}

	ipBlockListArr := strings.Split(string(ipBlockListContent), "\n")
	tmpIPBlockListCompiled := make(map[string]*net.IPNet)

	for ipBlockListLineNum, ipBlockListLine := range ipBlockListArr {
		ipBlockListLine = StrTrim(strings.SplitN(ipBlockListLine, "#", 2)[0])
		if ipBlockListLine == "" {
			Log("Debug-SetIPBlockListFromContent_Compile", GetLangText("Error-Debug-EmptyLine"), false, ipBlockListLineNum)
			continue
		}

		if cidr, exists := ipBlockListCompiled[ipBlockListLine]; exists {
			tmpIPBlockListCompiled[ipBlockListLine] = cidr
			continue
		}

		Log("Debug-SetIPBlockListFromContent_Compile", ":%d %s", false, ipBlockListLineNum, ipBlockListLine)
		cidr := ParseIPCIDR(ipBlockListLine)
		if cidr == nil {
			Log("SetIPBlockListFromContent_Compile", GetLangText("Error-SetIPBlockListFromContent_Compile"), true, ipBlockListLineNum, ipBlockListLine)
			continue
		}

		tmpIPBlockListCompiled[ipBlockListLine] = cidr
	}

	ipBlockListCompiled = tmpIPBlockListCompiled
	return len(ipBlockListCompiled)
}
func SetBlockListFromURL() bool {
	if config.BlockListURL == "" || (blockListURLLastFetch + int64(config.UpdateInterval)) > currentTimestamp {
		return true
	}

	blockListURLLastFetch = currentTimestamp

	_, _, blockListContent := Fetch(config.BlockListURL, false, false, nil)
	if blockListContent == nil {
		blockListURLLastFetch -= (int64(config.UpdateInterval) + 900)
		Log("SetBlockListFromURL", GetLangText("Error-FetchResponse2"), true)
		return false
	}

	ruleCount := SetBlockListFromContent(blockListContent, blockListFromURLCompiled)

	Log("SetBlockListFromURL", GetLangText("Success-SetBlockListFromURL"), true, ruleCount)

	return true
}
func SetIPBlockListFromURL() bool {
	if config.IPBlockListURL == "" || (ipBlockListURLLastFetch + int64(config.UpdateInterval)) > currentTimestamp {
		return true
	}

	ipBlockListURLLastFetch = currentTimestamp

	_, _, ipBlockListContent := Fetch(config.IPBlockListURL, false, false, nil)
	if ipBlockListContent == nil {
		ipBlockListURLLastFetch -= (int64(config.UpdateInterval) + 900)
		Log("SetIPBlockListFromURL", GetLangText("Error-FetchResponse2"), true)
		return false
	}

	ruleCount := SetIPBlockListFromContent(ipBlockListContent, ipBlockListFromURLCompiled)

	Log("SetIPBlockListFromURL", GetLangText("Success-SetIPBlockListFromURL"), true, ruleCount)

	return true
}
func SetBlockListFromFile() bool {
	if config.BlockListFile == "" {
		return true
	}

	blockListFileStat, err := os.Stat(config.BlockListFile)
	if err != nil {
		Log("SetBlockListFromFile", GetLangText("Error-LoadFile"), false, config.BlockListFile, err.Error())
		return false
	}

	tmpBlockListFileLastMod := blockListFileStat.ModTime().Unix()
	if tmpBlockListFileLastMod <= blockListFileLastMod {
		return false
	}

	if blockListFileLastMod != 0 {
		Log("Debug-SetBlockListFromFile", GetLangText("Debug-SetBlockListFromFile_HotReload"), false, config.BlockListFile)
	}

	blockListFile, err := os.ReadFile(config.BlockListFile)
	if err != nil {
		Log("SetBlockListFromFile", GetLangText("Error-LoadFile"), true, config.BlockListFile, err.Error())
		return false
	}

	blockListFileLastMod = tmpBlockListFileLastMod

	ruleCount := SetBlockListFromContent(blockListFile, blockListFromFileCompiled)

	Log("SetBlockListFromFile", GetLangText("Success-SetBlockListFromFile"), true, ruleCount)

	return true
}
func SetIPBlockListFromFile() bool {
	if config.IPBlockListFile == "" {
		return true
	}

	ipBlockListFileStat, err := os.Stat(config.IPBlockListFile)
	if err != nil {
		Log("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), false, config.IPBlockListFile, err.Error())
		return false
	}

	tmpIPBlockListFileLastMod := ipBlockListFileStat.ModTime().Unix()
	if tmpIPBlockListFileLastMod <= ipBlockListFileLastMod {
		return false
	}

	if ipBlockListFileLastMod != 0 {
		Log("Debug-SetIPBlockListFromFile", GetLangText("Debug-SetIPBlockListFromFile_HotReload"), false, config.IPBlockListFile)
	}

	ipBlockListFile, err := os.ReadFile(config.IPBlockListFile)
	if err != nil {
		Log("SetIPBlockListFromFile", GetLangText("Error-LoadFile"), true, config.IPBlockListFile, err.Error())
		return false
	}

	ipBlockListFileLastMod = tmpIPBlockListFileLastMod

	ruleCount := SetIPBlockListFromContent(ipBlockListFile, ipBlockListFromFileCompiled)

	Log("SetIPBlockListFromFile", GetLangText("Success-SetIPBlockListFromFile"), true, ruleCount)

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
		httpTransport.TLSClientConfig = &tls.Config { InsecureSkipVerify: true }
	} else {
		httpTransport.TLSClientConfig = &tls.Config { InsecureSkipVerify: false }
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

	httpClient = http.Client {
		Timeout:   currentTimeout,
		Jar:       cookieJar,
		Transport: httpTransport,
		CheckRedirect: func (req *http.Request, via []*http.Request) error {
	        return http.ErrUseLastResponse
	    },
	}

	httpClientWithoutCookie = http.Client {
		Timeout:   currentTimeout,
		Transport: httpTransportWithoutCookie,
		CheckRedirect: func (req *http.Request, via []*http.Request) error {
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

	blockListCompiled = make([]*regexp2.Regexp, len(config.BlockList))
	for k, v := range config.BlockList {
		Log("Debug-LoadConfig_CompileBlockList", "%s", false, v)

		reg, err := regexp2.Compile("(?i)" + v, 0)
		if err != nil {
			Log("LoadConfig_CompileBlockList", GetLangText("Error-CompileBlockList"), false, v)
			continue
		}

		reg.MatchTimeout = 50 * time.Millisecond

		blockListCompiled[k] = reg
	}

	ipBlockListCompiled = make([]*net.IPNet, len(config.IPBlockList))
	for k, v := range config.IPBlockList {
		Log("Debug-LoadConfig_CompileIPBlockList", "%s", false, v)

		cidr := ParseIPCIDR(v)
		if cidr == nil {
			Log("LoadConfig_CompileIPBlockList", GetLangText("Error-CompileIPBlockList"), false, v)
			continue
		}

		ipBlockListCompiled[k] = cidr
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
			loadAdditionalConfigStatus = LoadConfig("config/" + additionConfigFilename, false)
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
