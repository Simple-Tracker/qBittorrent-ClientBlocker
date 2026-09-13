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
		LogError("LoadLog", GetLangText("Error-LoadLog_Close"), false, err.Error())
		return false
	}

	logFile = nil
	return true
}

func Log(module string, str string, logToFile bool, args ...interface{}) {
	LogAtLevel(LogMessageLevel(module), module, str, logToFile, args...)
}

func LogError(module string, str string, logToFile bool, args ...interface{}) {
	LogAtLevel("error", module, str, logToFile, args...)
}

func LogAtLevel(level, module, str string, logToFile bool, args ...interface{}) {
	if !strings.HasPrefix(module, "Debug") {
		if module == "LogWriter" {
			str = StrTrim(str)
			if strings.HasPrefix(str, "[proxy.Provider") {
				return
			}
		}
	} else if ConfigSnapshot().Debug {
		if ConfigSnapshot().LogDebug {
			logToFile = true
		}
	} else {
		return
	}

	logStr := fmt.Sprintf("["+GetDateTime(true)+"]["+module+"] "+str+".\n", args...)
	if ConfigSnapshot().LogToFile && logToFile && logFile != nil {
		if _, err := logFile.Write([]byte(logStr)); err != nil {
			LogError("Log", GetLangText("Error-Log_Write"), false, err.Error())
		}
	}

	fmt.Print(logStr)
	if ConfigSnapshot().WebUI && module != "LoadConfig_Current" {
		logBufferMutex.Lock()
		logBuffer = append(logBuffer, logStr)
		if len(logBuffer) > logBufferMaxSize {
			logBuffer = logBuffer[1:]
		}
		logBufferMutex.Unlock()
		AppendWebUILog(level, module, fmt.Sprintf(str, args...))
	}
}
func LoadLog() bool {
	if !ConfigSnapshot().LogToFile || ConfigSnapshot().LogPath == "" {
		return false
	}

	if err := os.Mkdir(ConfigSnapshot().LogPath, os.ModePerm); err != nil && !os.IsExist(err) {
		LogError("LoadLog", GetLangText("Error-LoadLog_Mkdir"), false, err.Error())
		return false
	}

	tmpTodayStr := GetDateTime(false)
	newDay := (todayStr != tmpTodayStr)
	newLogPath := (lastLogPath != ConfigSnapshot().LogPath)

	if !newDay && !newLogPath {
		return true
	}

	if newDay {
		todayStr = tmpTodayStr
	}

	if newLogPath {
		if lastLogPath != "" {
			Log("LoadLog", GetLangText("LoadLog_HotReload"), false, ConfigSnapshot().LogPath)
		}
		lastLogPath = ConfigSnapshot().LogPath
	}

	tLogFile, err := os.OpenFile(ConfigSnapshot().LogPath+"/"+todayStr+".txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		LogError("LoadLog", GetLangText("Error-LoadLog_Open"), false, err.Error())
		return false
	}

	if !CloseLogFile() {
		tLogFile.Close()
		return false
	}

	logFile = tLogFile

	return true
}
