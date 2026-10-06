package app

import (
	"net/http"
	"net/url"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/app/i18n"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/app/logging"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/app/proxy"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/webui"
)

var language = i18n.NewTranslator()
var appLogger = NewAppLogger()
var systemProxy = proxy.NewResolver(Log, GetLangText)

func NewAppLogger() *logging.Logger {
	return logging.NewLogger(logging.Services{
		Settings: func() logging.Settings {
			cfg := ConfigSnapshot()
			return logging.Settings{Debug: cfg.Debug, LogDebug: cfg.LogDebug, LogToFile: cfg.LogToFile, LogPath: cfg.LogPath, WebUI: cfg.WebUI}
		},
		Text: GetLangText, Level: webui.LogMessageLevel, Publish: webui.AppendWebUILog,
	})
}

func LoadLang(code string) bool                  { return language.LoadLang(code, LogError) }
func GetLangCode() string                        { return i18n.GetLangCode() }
func GetLangText(key string) string              { return language.GetLangText(key) }
func GetProxy(r *http.Request) (*url.URL, error) { return systemProxy.GetProxy(r) }
func Log(module, message string, toFile bool, args ...any) {
	appLogger.Log(module, message, toFile, args...)
}
func LogError(module, message string, toFile bool, args ...any) {
	appLogger.LogError(module, message, toFile, args...)
}
func LogAtLevel(level, module, message string, toFile bool, args ...any) {
	appLogger.LogAtLevel(level, module, message, toFile, args...)
}
func LoadLog() bool      { return appLogger.LoadLog() }
func CloseLogFile() bool { return appLogger.CloseLogFile() }
func WriteCrashLog(location string, recoverErr any, stack []byte) {
	appLogger.WriteCrashLog(location, recoverErr, stack)
}
