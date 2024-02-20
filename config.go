package main

import (
	"os"
	"time"
	"flag"
	"regexp"
	"reflect"
	"strconv"
	"strings"
	"io/ioutil"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
)

type ConfigStruct struct {
	Debug                         bool
	Debug_CheckTorrent            bool
	Debug_CheckPeer               bool
	Interval                      uint32
	CleanInterval                 uint32
	PeerMapCleanInterval          uint32
	BanTime                       uint32
	BanAllPort                    bool
	StartDelay                    uint32
	SleepTime                     uint32
	Timeout                       uint32
	IPUploadedCheck               bool
	IPUpCheckInterval             uint32
	IPUpCheckIncrementMB          uint32
	MaxIPPortCount                uint32
	BanByProgressUploaded         bool
	BanByPUStartMB                uint32
	BanByPUStartPrecent           uint32
	BanByPUAntiErrorRatio         uint32
	BanByRelativeProgressUploaded bool
	BanByRelativePUStartMB        uint32
	BanByRelativePUStartPrecent   uint32
	BanByRelativePUAntiErrorRatio uint32
	LongConnection                bool
	LogPath                       string
	LogToFile                     bool
	LogDebug                      bool
	QBURL                         string
	QBUsername                    string
	QBPassword                    string
	UseBasicAuth                  bool
	SkipCertVerification          bool
	BlockList                     []string
}

var programVersion = "Unknown"
var shortFlag_ShowVersion bool
var longFlag_ShowVersion bool

var blockListCompiled []*regexp.Regexp
var cookieJar, _ = cookiejar.New(nil)

var lastQBURL = ""
var configFilename string
var configLastMod int64 = 0
var qBConfigLastMod int64 = 0

var httpTransport = &http.Transport {
	DisableKeepAlives:   false,
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
var config = ConfigStruct {
	Debug:                         false,
	Debug_CheckTorrent:            false,
	Debug_CheckPeer:               false,
	Interval:                      6,
	CleanInterval:                 3600,
	PeerMapCleanInterval:          60,
	BanTime:                       86400,
	BanAllPort:                    false,
	StartDelay:                    0,
	SleepTime:                     20,
	Timeout:                       6,
	IPUploadedCheck:               false,
	IPUpCheckInterval:             3600,
	IPUpCheckIncrementMB:          180000,
	MaxIPPortCount:                0,
	BanByProgressUploaded:         false,
	BanByPUStartMB:                10,
	BanByPUStartPrecent:           2,
	BanByPUAntiErrorRatio:         5,
	BanByRelativeProgressUploaded: false,
	BanByRelativePUStartMB:        10,
	BanByRelativePUStartPrecent:   2,
	BanByRelativePUAntiErrorRatio: 5,
	LongConnection:                true,
	LogPath:                       "logs",
	LogToFile:                     true,
	LogDebug:                      false,
	QBURL:                         "",
	QBUsername:                    "",
	QBPassword:                    "",
	UseBasicAuth:                  false,
	SkipCertVerification:          false,
	BlockList:                     []string {},
}

func GetQBConfigPath() string {
	var qBConfigFilename string
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		Log("Debug-GetQBConfigPath", "获取 User Home 目录时发生了错误: %s", false, err.Error())
		return ""
	}
	if !strings.Contains(userHomeDir, "\\") {
		qBConfigFilename = userHomeDir + "/.config/qBittorrent/qBittorrent.ini"
	} else {
		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			Log("Debug-GetQBConfigPath", "获取 User Config 目录时发生了错误: %s", false, err.Error())
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
	qBConfigFileStat, err := os.Stat(qBConfigFilename)
	if err != nil {
		if !os.IsNotExist(err) {
			// 避免反复猜测默认 qBittorrent 配置文件的失败信息影响 Debug 用户体验.
			Log("GetConfigFromQB", "读取 qBittorrent 配置文件元数据时发生了错误: %s", false, err.Error())
		}
		return []byte {}
	}
	tmpQBConfigLastMod := qBConfigFileStat.ModTime().Unix()
	if config.QBURL != "" && tmpQBConfigLastMod <= qBConfigLastMod {
		return []byte {}
	}
	Log("GetConfigFromQB", "使用 qBittorrent 配置文件: %s", false, qBConfigFilename)
	if qBConfigLastMod != 0 {
		Log("Debug-GetConfigFromQB", "发现 qBittorrent 配置文件更改, 正在进行热重载", false)
	}
	qBConfigFile, err := ioutil.ReadFile(qBConfigFilename)
	if err != nil {
		Log("GetConfigFromQB", "读取 qBittorrent 配置文件时发生了错误: %s", false, err.Error())
		return []byte {}
	}
	qBConfigLastMod = tmpQBConfigLastMod
	return qBConfigFile
}
func SetQBURLFromQB() bool {
	qBConfigFile := GetConfigFromQB()
	if len(qBConfigFile) < 1 {
		return false
	}
	qBConfigFileArr := strings.Split(string(qBConfigFile), "\n")
	qBWebUIEnabled := false
	qBHTTPSEnabled := false
	qBAddress := ""
	qBPort := 8080
	qBUsername := ""
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
				qBUsername = qbConfigLineArr[1]
		}
	}
	if !qBWebUIEnabled || qBAddress == "" {
		Log("SetQBURLFromQB", "放弃读取 qBittorrent 配置文件 (qBWebUIEnabled: %t, qBAddress: %s)", false, qBWebUIEnabled, qBAddress)
		return false
	}
	if qBHTTPSEnabled {
		config.QBURL = "https://" + qBAddress
		if qBPort != 443 {
			config.QBURL += ":" + strconv.Itoa(qBPort)
		}
	} else {
		config.QBURL = "http://" + qBAddress
		if qBPort != 80 {
			config.QBURL += ":" + strconv.Itoa(qBPort)
		}
	}
	config.QBUsername = qBUsername
	Log("SetQBURLFromQB", "读取 qBittorrent 配置文件成功 (qBWebUIEnabled: %t, qBURL: %s, qBUsername: %s)", false, qBWebUIEnabled, config.QBURL, config.QBUsername)
	return true
}
func LoadConfig() bool {
	configFileStat, err := os.Stat(configFilename)
	if err != nil {
		Log("Debug-LoadConfig", "读取配置文件元数据时发生了错误: %s", false, err.Error())
		return false
	}
	tmpConfigLastMod := configFileStat.ModTime().Unix()
	if tmpConfigLastMod <= configLastMod {
		return true
	}
	if configLastMod != 0 {
		Log("Debug-LoadConfig", "发现配置文件更改, 正在进行热重载", false)
	}
	configFile, err := ioutil.ReadFile(configFilename)
	if err != nil {
		Log("LoadConfig", "读取配置文件时发生了错误: %s", false, err.Error())
		return false
	}
	configLastMod = tmpConfigLastMod
	if err := json.Unmarshal(configFile, &config); err != nil {
		Log("LoadConfig", "解析配置文件时发生了错误: %s", false, err.Error())
		return false
	}
	Log("LoadConfig", "读取配置文件成功", true)
	InitConfig()
	return true
}
func InitConfig() {
	if config.LogToFile {
		LoadLog()
	} else if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	if config.Interval < 1 {
		config.Interval = 1
	}
	if config.Timeout < 1 {
		config.Timeout = 1
	}
	if config.QBURL != "" {
		config.QBURL = strings.TrimRight(config.QBURL, "/")
	}
	if config.SkipCertVerification {
		httpTransport.TLSClientConfig = &tls.Config { InsecureSkipVerify: true }
	} else {
		httpTransport.TLSClientConfig = &tls.Config { InsecureSkipVerify: false }
	}
	if !config.LongConnection {
		httpClient = http.Client {
			Timeout:   time.Duration(config.Timeout) * time.Second,
			Jar:       cookieJar,
			Transport: httpTransport,
		}
	} else if config.Timeout != 6 {
		httpClient = http.Client {
			Timeout:   time.Duration(config.Timeout) * time.Second,
			Jar:       cookieJar,
			Transport: httpTransport,
		}
	} else {
		httpClient = http.Client {
			Timeout:   6 * time.Second,
			Jar:       cookieJar,
			Transport: httpTransport,
		}
	}
	t := reflect.TypeOf(config)
	v := reflect.ValueOf(config)
	for k := 0; k < t.NumField(); k++ {
		Log("LoadConfig-Current", "%v: %v", true, t.Field(k).Name, v.Field(k).Interface())
	}
	blockListCompiled = make([]*regexp.Regexp, len(config.BlockList))
	for k, v := range config.BlockList {
		Log("Debug-LoadConfig-CompileBlockList", "%s", false, v)
		reg, err := regexp.Compile("(?i)" + v)
		if err != nil {
			Log("LoadConfig-CompileBlockList", "表达式 %s 有错误", true, v)
			continue
		}
		blockListCompiled[k] = reg
	}
}
func LoadInitConfig(firstLoad bool) {
	lastQBURL = config.QBURL
	if !LoadConfig() {
		Log("RunConsole", "读取配置文件失败或不完整", false)
		InitConfig()
	}
	if firstLoad && config.QBURL == "" {
		SetQBURLFromQB()
	}
	if config.QBURL != "" {
		if !firstLoad && lastQBURL != config.QBURL {
			SubmitBlockPeer("")
			lastQBURL = config.QBURL
		}
	} else {
		// 重置为上次使用的 QBURL, 主要目的是防止热重载配置文件可能破坏首次启动后从 qBittorrent 配置文件读取的 QBURL.
		config.QBURL = lastQBURL
	}
}
func ShowVersion() {
	Log("ShowVersion", "qBittorrent-ClientBlocker %s", false, programVersion)
}
func RegFlag() {
	flag.BoolVar(&shortFlag_ShowVersion, "v", false, "程序版本")
	flag.BoolVar(&longFlag_ShowVersion, "version", false, "程序版本")
	flag.StringVar(&configFilename, "c", "config.json", "配置文件路径")
	flag.StringVar(&configFilename, "config", "config.json", "配置文件路径")
	flag.BoolVar(&config.Debug, "debug", false, "调试模式")
	flag.Parse()
}
