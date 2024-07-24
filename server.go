package main

import (
	"context"
	"net"
	"net/http"
	"strings"
)

var Server_Status bool = false
var Server_httpListen net.Listener

type HttpServerHandler struct {
}

func (h *HttpServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	if Server_Status {
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

	Server_Status = true
	Server_httpListen = httpListen

	HttpServer.SetKeepAlivesEnabled(false)
	if err := HttpServer.Serve(Server_httpListen); err != http.ErrServerClosed {
		Log("StartServer", GetLangText("Error-StartServer_Serve"), true, err.Error())
		Server_Status = false
	}
}
func StopServer() {
	if !Server_Status {
		return
	}

	if err := HttpServer.Shutdown(context.Background()); err != nil {
		Log("StopServer", GetLangText("Error-StopServer"), true, err.Error())
	}

	// 出于保险起见, 再次关闭似乎已被 httpServer 同时关闭的 Listener, 但无视错误.
	Server_httpListen.Close()
}
