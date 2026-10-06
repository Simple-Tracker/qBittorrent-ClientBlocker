package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// GoWithCrashLog 在崩溃记录写入完成后关闭返回的通道.
func (l *Logger) GoWithCrashLog(location string, fn func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recoverErr := recover(); recoverErr != nil {
				l.WriteCrashLog(location, recoverErr, debug.Stack())
			}
		}()
		fn()
	}()
	return done
}

func (l *Logger) CrashLogPath() string {
	logPath := l.services.Settings().LogPath
	if logPath == "" {
		logPath = "logs"
	}

	return filepath.Join(logPath, "crash.log")
}

func (l *Logger) WriteCrashLog(location string, recoverErr any, recoverStack []byte) {
	l.crashLogMutex.Lock()
	defer l.crashLogMutex.Unlock()

	logPath := l.services.Settings().LogPath
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
