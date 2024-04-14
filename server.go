package main

import (
	"strings"
	"context"
	"net"
	"net/http"
)

var Server_Status bool = false
var Server_httpListen net.Listener

type httpServerHandler struct {
}

func (h *httpServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		w.Write([]byte("405: Method Not Allowed."))
		return
	}

	if currentClientType == "Transmission" && Tr_ProcessHTTP(w, r) {
		return
	}

	w.WriteHeader(404)
	w.Write([]byte("404: Not Found."))
}
func StartServer() {
	if Server_httpListen != nil {
		return
	}

	listenType := "tcp4"
	if IsIPv6(config.Listen) {
		listenType = "tcp6"
	}

	httpListen, err := net.Listen(listenType, strings.SplitN(config.Listen, "/", 2)[0])
	if err != nil {
	    Log("StartServer", GetLangText("Error-StartServer_Listen"), true, err.Error())
	    return
	}

	Server_httpListen = httpListen
	Server_Status = true

	httpServer.SetKeepAlivesEnabled(false)
	if err := httpServer.Serve(Server_httpListen); err != http.ErrServerClosed {
		Log("StartServer", GetLangText("Error-StartServer_Serve"), true, err.Error())
		Server_Status = false
	}
}
func StopServer() {
	if !Server_Status {
		return
	}

	if err := httpServer.Shutdown(context.Background()); err != nil {
		Log("StopServer", GetLangText("Error-StopServer"), true, err.Error())
	}

	// 出于保险起见, 再次关闭似乎已被 httpServer 同时关闭的 Listener, 但无视错误.
	Server_httpListen.Close()
}
