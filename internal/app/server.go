package app

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/webui"
)

var Server_Status bool = false
var Server_Listeners []net.Listener

type httpServerHandler struct {
}

func (h *httpServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	apiPath := strings.HasPrefix(r.URL.Path, "/api/")
	if apiPath {
		w.Header().Set("Cache-Control", "no-store")
	}
	if r.Method != "GET" {
		if apiPath {
			w.Header().Set("Allow", "GET")
			webui.WriteWebUIAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		w.WriteHeader(405)
		w.Write([]byte("405: Method Not Allowed."))
		return
	}

	// WebUI 路由.
	if ConfigSnapshot().WebUI {
		if webui.WebUI_IsPath(r.URL.Path) {
			if !webui.WebUI_CheckBasicAuth(w, r) {
				return
			}
			if r.URL.Path == "/" {
				webui.WebUI_Index(w, r)
				return
			}
			if r.URL.Path == "/api/v1/logs" {
				webui.WebUI_GetStructuredLogs(w, r)
				return
			}
			if r.URL.Path == "/api/v1/bans" {
				webui.WebUI_GetBans(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/bans/") {
				webui.WebUI_GetBan(w, r)
				return
			}
			if r.URL.Path == "/api/v1/status" {
				webui.WebUI_GetHealth(w, r)
				return
			}
			if r.URL.Path == "/api/status" {
				webui.WebUI_GetStatus(w, r)
				return
			}
			if r.URL.Path == "/api/peers" {
				webui.WebUI_GetPeers(w, r)
				return
			}
			if r.URL.Path == "/api/logs" {
				webui.WebUI_GetLogs(w, r)
				return
			}
			return
		}
	}

	if apiPath {
		webui.WriteWebUIAPIError(w, http.StatusNotFound, "not_found")
		return
	}

	// Transmission 兼容路由.
	activeClient, clientType := CurrentClientSnapshot()
	if handler, ok := activeClient.(client.BlocklistHandler); ok && clientType == "Transmission" && handler.ServeBlocklist(w, r) {
		return
	}

	w.WriteHeader(404)
	w.Write([]byte("404: Not Found."))
}

func CreateListener(addrStr string) (net.Listener, error) {
	listenType := "tcp4"
	if IsIPv6(addrStr) {
		listenType = "tcp6"
	}

	// 注意: Go 的 net.Listen("tcp", address) 在 address 为 ":7222" 或 "0.0.0.0:7222" 时,
	// 有可能在某些系统上自动启用 IPv6 双栈. 按照要求, 非 IPv6 地址强制使用 tcp4.
	addr := strings.SplitN(addrStr, "/", 2)[0]
	return net.Listen(listenType, addr)
}

func StartServer() {
	if Server_Status {
		return
	}

	// 收集需要监听的地址.
	var addrs []string
	if CurrentClientTypeSnapshot() == "Transmission" {
		addrs = append(addrs, ConfigSnapshot().Listen)
	}
	if ConfigSnapshot().WebUI {
		// 检查是否与 Transmission 监听地址冲突, 冲突则只监听一次.
		conflict := false
		for _, a := range addrs {
			if a == ConfigSnapshot().WebUIListen {
				conflict = true
				break
			}
		}
		if !conflict {
			addrs = append(addrs, ConfigSnapshot().WebUIListen)
		}
	}

	if len(addrs) == 0 {
		return
	}

	for _, addr := range addrs {
		l, err := CreateListener(addr)
		if err != nil {
			LogError("StartServer", GetLangText("Error-StartServer_Listen"), true, addr, err.Error())
			continue
		}
		Server_Listeners = append(Server_Listeners, l)
		Log("StartServer", GetLangText("StartServer_ListeningOn"), true, addr)
	}

	if len(Server_Listeners) == 0 {
		return
	}

	// Server 的超时配置仅在启动监听时设置, 避免与 Serve 并发修改.
	timeout := time.Duration(ConfigSnapshot().Timeout) * time.Second
	httpServer.ReadTimeout, httpServer.WriteTimeout = timeout, timeout
	Server_Status = true
	httpServer.SetKeepAlivesEnabled(false)

	for _, l := range Server_Listeners {
		ln := l
		GoWithCrashLog("httpServer.Serve", func() {
			if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
				LogError("StartServer", GetLangText("Error-StartServer_Serve"), true, ln.Addr().String(), err.Error())
			}
		})
	}
}

func StopServer() {
	if !Server_Status {
		return
	}

	if err := httpServer.Shutdown(context.Background()); err != nil {
		LogError("StopServer", GetLangText("Error-StopServer"), true, err.Error())
	}

	for _, l := range Server_Listeners {
		l.Close()
	}
	Server_Listeners = nil
	Server_Status = false
}
