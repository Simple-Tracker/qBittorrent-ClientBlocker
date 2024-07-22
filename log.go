package main

import (
	"io"
	"os"
	"fmt"
	"strings"
)

var todayStr = ""
var lastLogPath = ""
var logFile *os.File
var logwriter = LogWriter {}

type LogWriter struct {
    w io.Writer
}

func (w LogWriter) Write(p []byte) (n int, err error) {
	Log("LogWriter", string(p), true)
	return len(p), nil
}
func Log(module string, str string, logToFile bool, args ...interface {}) {
	if !strings.HasPrefix(module, "Debug") {
		if module == "LogWriter" {
			str = StrTrim(str)
			if strings.HasPrefix(str, "[proxy.Provider") {
				return
			}
		}
	} else if config.Debug {
		if config.LogDebug {
			logToFile = true
		}
	} else {
		return
	}

	logStr := fmt.Sprintf("[" + GetDateTime(true) + "][" + module + "] " + str + ".\n", args...)
	if config.LogToFile && logToFile && logFile != nil {
		if _, err := logFile.Write([]byte(logStr)); err != nil {
			Log("Log", GetLangText("Error-Log_Write"), false, err.Error())
		}
	}

	fmt.Print(logStr)
}
func LoadLog() bool {
	if !config.LogToFile || config.LogPath == "" {
		return false
	}

	if err := os.Mkdir(config.LogPath, os.ModePerm); err != nil && !os.IsExist(err) {
		Log("LoadLog", GetLangText("Error-LoadLog_Mkdir"), false, err.Error())
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
			Log("LoadLog", GetLangText("LoadLog_HotReload"), false, config.LogPath)
		}
		lastLogPath = config.LogPath
	}

	tLogFile, err := os.OpenFile(config.LogPath + "/" + todayStr + ".txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		tLogFile.Close()
		tLogFile = nil
		Log("LoadLog", GetLangText("Error-LoadLog_Close"), false, err.Error())
		return false
	}

	logFile.Close()
	logFile = tLogFile

	return true
}
