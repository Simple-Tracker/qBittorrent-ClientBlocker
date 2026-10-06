package app

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/app/platform"
)

func Run(version string) {
	SetVersion(version)
	location := "main"
	if runtime.GOOS == "windows" {
		location = "main_windows"
	}
	defer RecoverAndStop(location, true)
	if PrepareEnv() {
		platform.Start(platform.Options{
			ProgramName: programName,
			RegHotKey:   needRegHotKey, HideWindow: needHideWindow, HideSystray: needHideSystray,
			Log: Log, LogError: LogError, Text: GetLangText,
			Recover: RecoverAndStop, Go: GoWithCrashLog, RequestStop: ReqStop,
		})
		RunConsole()
	}
}

// SetVersion 同时更新由注入的构建版本生成的用户代理字符串.
func SetVersion(version string) {
	programVersion = version
	programUserAgent = fmt.Sprintf("%s/%s (%s, %s)", programName, version, runtime.GOOS, runtime.GOARCH)
	btnUserAgent = programUserAgent + " " + btnProtocol
}

var crashStopOnce sync.Once

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
	appLogger.GoWithCrashLog(location, fn)
}
