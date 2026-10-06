package logging

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Settings struct {
	Debug, LogDebug, LogToFile, WebUI bool
	LogPath                           string
}

type Services struct {
	Settings func() Settings
	Text     func(string) string
	Level    func(string) string
	Publish  func(level, module, message string)
}

// Logger 保存文件句柄和有界日志缓冲; 不依赖应用配置或 WebUI 实现.
type Logger struct {
	services              Services
	todayStr, lastLogPath string
	logFile               *os.File
	logBuffer             []string
	logBufferMaxSize      int
	logBufferMutex        sync.Mutex
	crashLogMutex         sync.Mutex
}

func NewLogger(services Services) *Logger {
	if services.Settings == nil {
		services.Settings = func() Settings { return Settings{} }
	}
	if services.Text == nil {
		services.Text = func(key string) string { return key }
	}
	if services.Level == nil {
		services.Level = func(string) string { return "info" }
	}
	if services.Publish == nil {
		services.Publish = func(string, string, string) {}
	}
	return &Logger{services: services, logBufferMaxSize: 100}
}

func dateTime(withTime bool) string {
	format := "2006-01-02"
	if withTime {
		format += " 15:04:05"
	}
	return time.Now().Format(format)
}

func (l *Logger) LegacyLogs() []string {
	l.logBufferMutex.Lock()
	defer l.logBufferMutex.Unlock()
	return append([]string{}, l.logBuffer...)
}

func (l *Logger) Write(p []byte) (n int, err error) {
	l.Log("LogWriter", string(p), true)
	return len(p), nil
}

func (l *Logger) CloseLogFile() bool {
	if l.logFile == nil {
		return true
	}

	if err := l.logFile.Close(); err != nil {
		l.LogError("LoadLog", l.services.Text("Error-LoadLog_Close"), false, err.Error())
		return false
	}

	l.logFile = nil
	return true
}

func (l *Logger) Log(module string, str string, logToFile bool, args ...interface{}) {
	l.LogAtLevel(l.services.Level(module), module, str, logToFile, args...)
}

func (l *Logger) LogError(module string, str string, logToFile bool, args ...interface{}) {
	l.LogAtLevel("error", module, str, logToFile, args...)
}

func (l *Logger) LogAtLevel(level, module, str string, logToFile bool, args ...interface{}) {
	if !strings.HasPrefix(module, "Debug") {
		if module == "LogWriter" {
			str = strings.Trim(str, "  \n\r")
			if strings.HasPrefix(str, "[proxy.Provider") {
				return
			}
		}
	} else if l.services.Settings().Debug {
		if l.services.Settings().LogDebug {
			logToFile = true
		}
	} else {
		return
	}

	logStr := fmt.Sprintf("["+dateTime(true)+"]["+module+"] "+str+".\n", args...)
	if l.services.Settings().LogToFile && logToFile && l.logFile != nil {
		if _, err := l.logFile.Write([]byte(logStr)); err != nil {
			l.LogError("Log", l.services.Text("Error-Log_Write"), false, err.Error())
		}
	}

	fmt.Print(logStr)
	if l.services.Settings().WebUI && module != "LoadConfig_Current" {
		l.logBufferMutex.Lock()
		l.logBuffer = append(l.logBuffer, logStr)
		if len(l.logBuffer) > l.logBufferMaxSize {
			l.logBuffer = l.logBuffer[1:]
		}
		l.logBufferMutex.Unlock()
		l.services.Publish(level, module, fmt.Sprintf(str, args...))
	}
}
func (l *Logger) LoadLog() bool {
	if !l.services.Settings().LogToFile || l.services.Settings().LogPath == "" {
		return false
	}

	if err := os.Mkdir(l.services.Settings().LogPath, os.ModePerm); err != nil && !os.IsExist(err) {
		l.LogError("LoadLog", l.services.Text("Error-LoadLog_Mkdir"), false, err.Error())
		return false
	}

	tmpTodayStr := dateTime(false)
	newDay := (l.todayStr != tmpTodayStr)
	newLogPath := (l.lastLogPath != l.services.Settings().LogPath)

	if !newDay && !newLogPath {
		return true
	}

	if newDay {
		l.todayStr = tmpTodayStr
	}

	if newLogPath {
		if l.lastLogPath != "" {
			l.Log("LoadLog", l.services.Text("LoadLog_HotReload"), false, l.services.Settings().LogPath)
		}
		l.lastLogPath = l.services.Settings().LogPath
	}

	tLogFile, err := os.OpenFile(l.services.Settings().LogPath+"/"+l.todayStr+".txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		l.LogError("LoadLog", l.services.Text("Error-LoadLog_Open"), false, err.Error())
		return false
	}

	if !l.CloseLogFile() {
		tLogFile.Close()
		return false
	}

	l.logFile = tLogFile

	return true
}
