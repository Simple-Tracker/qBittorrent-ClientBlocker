package main

import (
	"context"
	"net"
	"net/http"
	"strings"
)

var Server_Status bool = false
var Server_Listeners []net.Listener

type httpServerHandler struct {
}

func (h *httpServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		w.Write([]byte("405: Method Not Allowed."))
		return
	}

	// WebUI 路由.
	if config.WebUI {
		if WebUI_IsPath(r.URL.Path) {
			if !WebUI_CheckBasicAuth(w, r) {
				return
			}
			if r.URL.Path == "/" {
				WebUI_Index(w, r)
				return
			}
			if r.URL.Path == "/api/status" {
				WebUI_GetStatus(w, r)
				return
			}
			if r.URL.Path == "/api/peers" {
				WebUI_GetPeers(w, r)
				return
			}
			if r.URL.Path == "/api/logs" {
				WebUI_GetLogs(w, r)
				return
			}
			return
		}
	}

	// Transmission 兼容路由.
	if currentClientType == "Transmission" && Tr_ProcessHTTP(w, r) {
		return
	}

	w.WriteHeader(404)
	w.Write([]byte("404: Not Found."))
}

func createListener(addrStr string) (net.Listener, error) {
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
	if currentClientType == "Transmission" {
		addrs = append(addrs, config.Listen)
	}
	if config.WebUI {
		// 检查是否与 Transmission 监听地址冲突, 冲突则只监听一次.
		conflict := false
		for _, a := range addrs {
			if a == config.WebUIListen {
				conflict = true
				break
			}
		}
		if !conflict {
			addrs = append(addrs, config.WebUIListen)
		}
	}

	if len(addrs) == 0 {
		return
	}

	for _, addr := range addrs {
		l, err := createListener(addr)
		if err != nil {
			Log("StartServer", GetLangText("Error-StartServer_Listen"), true, addr, err.Error())
			continue
		}
		Server_Listeners = append(Server_Listeners, l)
		Log("StartServer", GetLangText("StartServer_ListeningOn"), true, addr)
	}

	if len(Server_Listeners) == 0 {
		return
	}

	Server_Status = true
	httpServer.SetKeepAlivesEnabled(false)

	for _, l := range Server_Listeners {
		ln := l
		GoWithCrashLog("httpServer.Serve", func() {
			if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
				Log("StartServer", GetLangText("Error-StartServer_Serve"), true, ln.Addr().String(), err.Error())
			}
		})
	}
}

func StopServer() {
	if !Server_Status {
		return
	}

	if err := httpServer.Shutdown(context.Background()); err != nil {
		Log("StopServer", GetLangText("Error-StopServer"), true, err.Error())
	}

	for _, l := range Server_Listeners {
		l.Close()
	}
	Server_Listeners = nil
	Server_Status = false
}
