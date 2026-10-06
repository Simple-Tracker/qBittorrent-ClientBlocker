package client

import (
	"net/http"
	"strings"
	"time"
)

// Settings 保存下载客户端协议所需选项的当前快照.
type Settings struct {
	URL               string
	Username          string
	Password          string
	BanAllPort        bool
	NewBanPeersMethod bool
	BlocklistURL      string
}

// Services 注入应用策略, 不暴露应用状态或锁.
type Services struct {
	Snapshot    func() Settings
	SetEndpoint func(url, username string)
	Fetch       func(url string, tryLogin, clientRequest, allowCache bool, headers *map[string]string) (int, http.Header, []byte)
	Submit      func(url string, body any, tryLogin, clientRequest bool, headers *map[string]string) (int, http.Header, []byte)
	Now         func() int64
	Log         func(module, message string, toFile bool, args ...any)
	LogError    func(module, message string, toFile bool, args ...any)
	Text        func(key string) string
}

func (s Services) WithDefaults() Services {
	if s.Snapshot == nil {
		s.Snapshot = func() Settings { return Settings{} }
	}
	if s.SetEndpoint == nil {
		s.SetEndpoint = func(string, string) {}
	}
	if s.Now == nil {
		s.Now = func() int64 { return time.Now().Unix() }
	}
	if s.Log == nil {
		s.Log = func(string, string, bool, ...any) {}
	}
	if s.LogError == nil {
		s.LogError = func(string, string, bool, ...any) {}
	}
	if s.Text == nil {
		s.Text = func(key string) string { return key }
	}
	return s
}

func IsIPv6(ip string) bool    { return strings.Count(ip, ":") >= 2 }
func Trim(value string) string { return strings.Trim(value, "  \n\r") }
