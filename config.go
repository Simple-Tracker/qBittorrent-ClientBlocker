package main

import (
	"os"
	"net"
	"time"
	"flag"
	"regexp"
	"reflect"
	"strconv"
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
	IgnoreEmptyPeer               bool
	IgnorePTTorrent               bool
	StartDelay                    uint32
	SleepTime                     uint32
	Timeout                       uint32
	LongConnection                bool
	LogPath                       string
	LogToFile                     bool
	LogDebug                      bool
	URL                           string
	Username                      string
	Password                      string
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
var httpClient = http.Client {
	Timeout:   6 * time.Second,
	Jar:       cookieJar,
	Transport: httpTransport,
}
var httpClientWithoutCookie = http.Client {
	Timeout:   6 * time.Second,
	Transport: httpTransport,
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
	IgnoreEmptyPeer:               true,
	IgnorePTTorrent:               true,
	StartDelay:                    0,
	SleepTime:                     20,
	Timeout:                       6,
	LongConnection:                true,
	LogPath:                       "logs",
	LogToFile:                     true,
	LogDebug:                      false,
	URL:                           "",
	Username:                      "",
	Password:                      "",
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

	ipfilter := Fetch(config.IPFilterURL, false, false)
	if ipfilter == nil {
		Log("SetIPFilter", GetLangText("Error-SetIPFilter_Fetch"), true)
		return false
	}

	// Max 8MB.
	if len(ipfilter) > 8388608 {
		Log("SetIPFilter", GetLangText("Error-SetIPFilter_LargeFile"), true)
		return false
	}

	ipfilterArr := strings.Split(string(ipfilter), "\n")
	ipfilterCompiled = make([]*net.IPNet, len(ipfilterArr))
	k := 0
	for ipfilterLineNum, ipfilterLine := range ipfilterArr {
		ipfilterLine = StrTrim(ipfilterLine)
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
func GetQBConfigPath() string {
	var qBConfigFilename string
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		Log("Debug-GetQBConfigPath", GetLangText("Error-Debug-GetQBConfigPath_GetUserHomeDir"), false, err.Error())
		return ""
	}
	if IsUnix(userHomeDir) {
		qBConfigFilename = userHomeDir + "/.config/qBittorrent/qBittorrent.ini"
	} else {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			Log("Debug-GetQBConfigPath", GetLangText("Error-Debug-GetQBConfigPath_GetUserConfigDir"), false, err.Error())
			return ""
		}
		qBConfigFilename = userConfigDir + "\\qBittorrent\\qBittorrent.ini"
	}
	return qBConfigFilename
}
func GetConfigFromQB() []byte {
	qBConfigFilename := GetQBConfigPath()
	if qBConfigFilename == "" {
		return []byte {}
	}

	_, err := os.Stat(qBConfigFilename)
	if err != nil {
		if !os.IsNotExist(err) {
			// 避免反复猜测默认 qBittorrent 配置文件的失败信息影响 Debug 用户体验.
			Log("GetConfigFromQB", GetLangText("Error-GetConfigFromQB_LoadConfigMeta"), false, err.Error())
		}
		return []byte {}
	}

	Log("GetConfigFromQB", GetLangText("GetConfigFromQB_UseQBConfig"), false, qBConfigFilename)

	qBConfigFile, err := os.ReadFile(qBConfigFilename)
	if err != nil {
		Log("GetConfigFromQB", GetLangText("Error-GetConfigFromQB_LoadConfig"), false, err.Error())
		return []byte {}
	}

	return qBConfigFile
}
func SetURLFromQB() bool {
	qBConfigFile := GetConfigFromQB()
	if len(qBConfigFile) < 1 {
		return false
	}
	qBConfigFileArr := strings.Split(string(qBConfigFile), "\n")
	qBWebUIEnabled := false
	qBHTTPSEnabled := false
	qBAddress := ""
	qBPort := 8080
	Username := ""
	for _, qbConfigLine := range qBConfigFileArr {
		qbConfigLineArr := strings.SplitN(qbConfigLine, "=", 2)
		if len(qbConfigLineArr) < 2 || qbConfigLineArr[1] == "" {
			continue
		}
		qbConfigLineArr[0] = strings.ToLower(StrTrim(qbConfigLineArr[0]))
		qbConfigLineArr[1] = strings.ToLower(StrTrim(qbConfigLineArr[1]))
		switch qbConfigLineArr[0] {
			case "webui\\enabled":
				if qbConfigLineArr[1] == "true" {
					qBWebUIEnabled = true
				}
			case "webui\\https\\enabled":
				if qbConfigLineArr[1] == "true" {
					qBHTTPSEnabled = true
				}
			case "webui\\address":
				if qbConfigLineArr[1] == "*" || qbConfigLineArr[1] == "0.0.0.0" {
					qBAddress = "127.0.0.1"
				} else if qbConfigLineArr[1] == "::" || qbConfigLineArr[1] == "::1" {
					qBAddress = "[::1]"
				} else {
					qBAddress = qbConfigLineArr[1]
				}
			case "webui\\port":
				tmpQBPort, err := strconv.Atoi(qbConfigLineArr[1])
				if err == nil {
					qBPort = tmpQBPort
				}
			case "webui\\username":
				Username = qbConfigLineArr[1]
		}
	}
	if !qBWebUIEnabled || qBAddress == "" {
		Log("SetURLFromQB", GetLangText("Abandon-SetURLFromQB"), false, qBWebUIEnabled, qBAddress)
		return false
	}
	if qBHTTPSEnabled {
		config.URL = "https://" + qBAddress
		if qBPort != 443 {
			config.URL += ":" + strconv.Itoa(qBPort)
		}
	} else {
		config.URL = "http://" + qBAddress
		if qBPort != 80 {
			config.URL += ":" + strconv.Itoa(qBPort)
		}
	}
	config.Username = Username
	Log("SetURLFromQB", GetLangText("Success-SetURLFromQB"), false, qBWebUIEnabled, config.URL, config.Username)
	return true
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

	if config.URL != "" {
		config.URL = strings.TrimRight(config.URL, "/")
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

	httpClient = http.Client {
		Timeout:   time.Duration(config.Timeout) * time.Second,
		Jar:       cookieJar,
		Transport: httpTransport,
	}

	httpClientWithoutCookie = http.Client {
		Timeout:   time.Duration(config.Timeout) * time.Second,
		Transport: httpTransportWithoutCookie,
	}

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
	lastURL = config.URL

	if !LoadConfig() {
		Log("LoadInitConfig", GetLangText("Failed-LoadInitConfig"), false)
		InitConfig()
	}

	if firstLoad && config.URL == "" {
		SetURLFromQB()
	}

	if config.URL != "" {
		if lastURL != config.URL {
			if firstLoad && !Login() {
				return false
			}
			SubmitBlockPeer(nil)
			lastURL = config.URL
		}
	} else {
		// 重置为上次使用的 URL, 主要目的是防止热重载配置文件可能破坏首次启动后从 qBittorrent 配置文件读取的 URL.
		config.URL = lastURL
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
