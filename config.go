package main

import (
	"os"
	"net"
	"time"
	"flag"
	"regexp"
	"reflect"
	"strings"
	"crypto/tls"
	"encoding/json"
	"path/filepath"
	"net/http"
	"net/http/cookiejar"
	"github.com/tidwall/jsonc"
)

type ConfigStruct struct {
	Debug                         bool
	Debug_CheckTorrent            bool
	Debug_CheckPeer               bool
	Interval                      uint32
	CleanInterval                 uint32
	TorrentMapCleanInterval       uint32
	BanTime                       uint32
	BanAllPort                    bool
	BanIPCIDR                     string
	BanIP6CIDR                    string
	IgnoreEmptyPeer               bool
	IgnorePTTorrent               bool
	StartDelay                    uint32
	SleepTime                     uint32
	Timeout                       uint32
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
	BlockList                     []string
	IPBlockList                   []string
	IPFilterURL                   string
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
var shortFlag_ShowVersion bool
var longFlag_ShowVersion bool
var noChdir bool

var randomStrRegexp = regexp.MustCompile("[a-zA-Z0-9]{32}")
var blockListCompiled []*regexp.Regexp
var ipBlockListCompiled []*net.IPNet
var ipfilterCompiled []*net.IPNet
var cookieJar, _ = cookiejar.New(nil)

var lastURL = ""
var configFilename string
var configLastMod int64 = 0
var ipfilterLastFetch int64 = 0

var httpTransport = &http.Transport {
	DisableKeepAlives:   true,
	ForceAttemptHTTP2:   false,
	MaxConnsPerHost:     32,
	MaxIdleConns:        32,
	MaxIdleConnsPerHost: 32,
	TLSClientConfig:     &tls.Config { InsecureSkipVerify: false },
}
var httpClient http.Client
var httpClientWithoutCookie http.Client
var httpServer = http.Server {
	ReadTimeout:  30,
	WriteTimeout: 30,
	Handler:      &httpServerHandler {},
}


var config = ConfigStruct {
	Debug:                         false,
	Debug_CheckTorrent:            false,
	Debug_CheckPeer:               false,
	Interval:                      6,
	CleanInterval:                 3600,
	TorrentMapCleanInterval:       60,
	BanTime:                       86400,
	BanAllPort:                    false,
	BanIPCIDR:                     "/32",
	BanIP6CIDR:                    "/128",
	IgnoreEmptyPeer:               true,
	IgnorePTTorrent:               true,
	StartDelay:                    0,
	SleepTime:                     20,
	Timeout:                       6,
	LongConnection:                true,
	LogPath:                       "logs",
	LogToFile:                     true,
	LogDebug:                      false,
	Listen:                        ":26262",
	ClientType:                    "",
	ClientURL:                     "",
	ClientUsername:                "",
	ClientPassword:                "",
	UseBasicAuth:                  false,
	SkipCertVerification:          false,
	BlockList:                     []string {},
	IPBlockList:                   []string {},
	IPFilterURL:                   "",
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
func SetIPFilter() bool {
	if config.IPFilterURL == "" || (ipfilterLastFetch + 86400) > currentTimestamp {
		return true
	}

	_, ipfilterContent := Fetch(config.IPFilterURL, false, false)
	if ipfilterContent == nil {
		Log("SetIPFilter", GetLangText("Error-SetIPFilter_Fetch"), true)
		return false
	}

	// Max 8MB.
	if len(ipfilterContent) > 8388608 {
		Log("SetIPFilter", GetLangText("Error-SetIPFilter_LargeFile"), true)
		return false
	}

	ipfilterArr := strings.Split(string(ipfilterContent), "\n")
	ipfilterCompiled = make([]*net.IPNet, len(ipfilterArr))
	k := 0
	for ipfilterLineNum, ipfilterLine := range ipfilterArr {
		ipfilterLine = StrTrim(strings.SplitN(ipfilterLine, "#", 2)[0])
		if ipfilterLine == "" {
			Log("Debug-SetIPFilter_Compile", GetLangText("Error-Debug-SetIPFilter_EmptyLine"), false, ipfilterLineNum)
			continue
		}

		Log("Debug-SetIPFilter_Compile", ":%d %s", false, ipfilterLineNum, ipfilterLine)
		cidr := ParseIP(ipfilterLine)
		if cidr == nil {
			Log("SetIPFilter_Compile", GetLangText("Error-SetIPFilter_Compile"), true, ipfilterLineNum, ipfilterLine)
			continue
		}

		ipfilterCompiled[k] = cidr

		k++
	}

	ipfilterLastFetch = currentTimestamp
	ruleCount := len(ipfilterCompiled)

	Log("SetIPFilter", GetLangText("Success-SetIPFilter"), true, ruleCount)

	if ruleCount > 0 {
		return true
	}

	return false
}
func LoadConfig() bool {
	configFileStat, err := os.Stat(configFilename)
	if err != nil {
		Log("Debug-LoadConfig", GetLangText("Error-LoadConfigMeta"), false, err.Error())
		return false
	}

	tmpConfigLastMod := configFileStat.ModTime().Unix()
	if tmpConfigLastMod <= configLastMod {
		return true
	}

	if configLastMod != 0 {
		Log("Debug-LoadConfig", GetLangText("Debug-LoadConfig_HotReload"), false)
	}

	configFile, err := os.ReadFile(configFilename)
	if err != nil {
		Log("LoadConfig", GetLangText("Error-LoadConfig"), false, err.Error())
		return false
	}

	configLastMod = tmpConfigLastMod

	if err := json.Unmarshal(jsonc.ToJSON(configFile), &config); err != nil {
		Log("LoadConfig", GetLangText("Error-ParseConfig"), false, err.Error())
		return false
	}

	Log("LoadConfig", GetLangText("Success-LoadConfig"), true)
	InitConfig()

	return true
}
func InitConfig() {
	if !LoadLog() && logFile != nil {
		logFile.Close()
		logFile = nil
	}

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
		Log("LoadConfig_Current", "%v: %v", true, t.Field(k).Name, v.Field(k).Interface())
	}

	blockListCompiled = make([]*regexp.Regexp, len(config.BlockList))
	for k, v := range config.BlockList {
		Log("Debug-LoadConfig_CompileBlockList", "%s", false, v)

		reg, err := regexp.Compile("(?i)" + v)
		if err != nil {
			Log("LoadConfig_CompileBlockList", GetLangText("Error-CompileBlockList"), true, v)
			continue
		}

		blockListCompiled[k] = reg
	}

	ipBlockListCompiled = make([]*net.IPNet, len(config.IPBlockList))
	for k, v := range config.IPBlockList {
		Log("Debug-LoadConfig_CompileIPBlockList", "%s", false, v)

		cidr := ParseIP(v)
		if cidr == nil {
			Log("LoadConfig_CompileIPBlockList", GetLangText("Error-CompileIPBlockList"), true, v)
			continue
		}

		ipBlockListCompiled[k] = cidr
	}
}
func LoadInitConfig(firstLoad bool) bool {
	lastURL = config.ClientURL

	if !LoadConfig() {
		Log("LoadInitConfig", GetLangText("Failed-LoadInitConfig"), false)
		InitConfig()
	}

	if firstLoad && config.ClientURL == "" {
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
		SetIPFilter()
	}

	return true
}
func RegFlag() {
	flag.BoolVar(&shortFlag_ShowVersion, "v", false, GetLangText("ProgramVersion"))
	flag.BoolVar(&longFlag_ShowVersion, "version", false, GetLangText("ProgramVersion"))
	flag.StringVar(&configFilename, "c", "config.json", GetLangText("ConfigPath"))
	flag.StringVar(&configFilename, "config", "config.json", GetLangText("ConfigPath"))
	flag.BoolVar(&config.Debug, "debug", false, GetLangText("DebugMode"))
	flag.BoolVar(&noChdir, "nochdir", false, GetLangText("NoChdir"))
	flag.Parse()
}
func ShowVersion() {
	Log("ShowVersion", "%s %s", false, programName, programVersion)
}
func PrepareEnv() bool {
	LoadLang(GetLangCode())
	RegFlag()
	ShowVersion()

	if shortFlag_ShowVersion || longFlag_ShowVersion {
		return false
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
