package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

var crashLogMutex sync.Mutex
var crashStopOnce sync.Once

func CrashLogPath() string {
	logPath := config.LogPath
	if logPath == "" {
		logPath = "logs"
	}

	return filepath.Join(logPath, "crash.log")
}

func WriteCrashLog(location string, recoverErr any, recoverStack []byte) {
	crashLogMutex.Lock()
	defer crashLogMutex.Unlock()

	logPath := config.LogPath
	if logPath == "" {
		logPath = "logs"
	}

	if err := os.MkdirAll(logPath, os.ModePerm); err != nil {
		fmt.Fprintf(os.Stderr, "[%s][Crash] failed to create crash log directory: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}

	crashFile, err := os.OpenFile(filepath.Join(logPath, "crash.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s][Crash] failed to open crash log: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}
	defer crashFile.Close()

	_, _ = fmt.Fprintf(crashFile, "[%s][%s] panic: %v\n", time.Now().Format("2006-01-02 15:04:05"), location, recoverErr)
	if len(recoverStack) > 0 {
		_, _ = crashFile.Write(recoverStack)
		if recoverStack[len(recoverStack)-1] != '\n' {
			_, _ = crashFile.Write([]byte("\n"))
		}
	}
	_, _ = crashFile.Write([]byte("\n"))
}

func RecoverAndStop(location string, isFatal bool) {
	if recoverErr := recover(); recoverErr != nil {
		recoverStack := debug.Stack()
		if isFatal {
			crashStopOnce.Do(func() {
				WriteCrashLog(location, recoverErr, recoverStack)
				Stop(recoverErr, recoverStack)
			})
		} else {
			WriteCrashLog(location, recoverErr, recoverStack)
		}
	}
}

func GoWithCrashLog(location string, fn func()) {
	go func() {
		defer RecoverAndStop(location, false)
		fn()
	}()
}
