package main

import (
	"os"
	"fmt"
	"strings"
)

var todayStr = ""
var lastLogPath = ""
var logFile *os.File

func Log(module string, str string, logToFile bool, args ...interface {}) {
	if strings.HasPrefix(module, "Debug") {
		if !config.Debug {
			return
		} else if config.LogDebug {
			logToFile = true
		}
	}
	logStr := fmt.Sprintf("[" + GetDateTime(true) + "][" + module + "] " + str + ".\n", args...)
	if config.LogToFile && logToFile && logFile != nil {
		if _, err := logFile.Write([]byte(logStr)); err != nil {
			Log("Log", "写入日志时发生了错误: %s", false, err.Error())
		}
	}
	fmt.Print(logStr)
}
func LoadLog() bool {
	if config.LogPath == "" {
		return false
	}
	if err := os.Mkdir(config.LogPath, os.ModePerm); err != nil && !os.IsExist(err) {
		Log("LoadLog", "创建日志目录时发生了错误: %s", false, err.Error())
		return false
	}

	tmpTodayStr := GetDateTime(false)
	newDay := (todayStr != tmpTodayStr)
	newLogPath := (lastLogPath != config.LogPath)

	if !newDay && !newLogPath {
		return true
	}
	if newDay {
		todayStr = tmpTodayStr
	}
	if newLogPath {
		if lastLogPath != "" {
			Log("LoadLog", "发现日志目录更改, 正在进行热重载 (%s)", false, config.LogPath)
		}
		lastLogPath = config.LogPath
	}

	tLogFile, err := os.OpenFile(config.LogPath + "/" + todayStr + ".txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		tLogFile.Close()
		tLogFile = nil
		Log("LoadLog", "访问日志时发生了错误: %s", false, err.Error())
		return false
	}
	logFile.Close()
	logFile = tLogFile

	return true
}
