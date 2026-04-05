package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

var todayStr = ""
var lastLogPath = ""
var logFile *os.File
var logwriter = LogWriter{}
var logBuffer []string
var logBufferMaxSize = 100
var logBufferMutex sync.Mutex

type LogWriter struct {
	w io.Writer
}

func (w LogWriter) Write(p []byte) (n int, err error) {
	Log("LogWriter", string(p), true)
	return len(p), nil
}

func CloseLogFile() bool {
	if logFile == nil {
		return true
	}

	if err := logFile.Close(); err != nil {
		Log("LoadLog", GetLangText("Error-LoadLog_Close"), false, err.Error())
		return false
	}

	logFile = nil
	return true
}

func Log(module string, str string, logToFile bool, args ...interface{}) {
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

	logStr := fmt.Sprintf("["+GetDateTime(true)+"]["+module+"] "+str+".\n", args...)
	if config.LogToFile && logToFile && logFile != nil {
		if _, err := logFile.Write([]byte(logStr)); err != nil {
			Log("Log", GetLangText("Error-Log_Write"), false, err.Error())
		}
	}

	fmt.Print(logStr)
	if config.WebUI && module != "LoadConfig_Current" {
		logBufferMutex.Lock()
		logBuffer = append(logBuffer, logStr)
		if len(logBuffer) > logBufferMaxSize {
			logBuffer = logBuffer[1:]
		}
		logBufferMutex.Unlock()
	}
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

	tLogFile, err := os.OpenFile(config.LogPath+"/"+todayStr+".txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		Log("LoadLog", GetLangText("Error-LoadLog_Open"), false, err.Error())
		return false
	}

	if !CloseLogFile() {
		tLogFile.Close()
		return false
	}

	logFile = tLogFile

	return true
}
