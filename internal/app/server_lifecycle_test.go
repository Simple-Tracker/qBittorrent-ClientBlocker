package app

import (
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/transmission"
)

func serverTestSetup(t *testing.T) {
	t.Helper()
	oldConfig := ConfigSnapshot()
	oldClient, oldType := CurrentClientSnapshot()
	t.Cleanup(func() {
		StopServer()
		ReplaceConfig(oldConfig)
		SetCurrentClient(oldClient, oldType)
	})
	cfg := *oldConfig
	cfg.WebUI = true
	cfg.WebUIUsername = ""
	cfg.WebUIPassword = ""
	cfg.LogToFile = false
	cfg.Listen = "127.0.0.1:0"
	cfg.WebUIListen = "localhost:0"
	ReplaceConfig(&cfg)
	SetCurrentClient(transmission.New(ClientServices()), "Transmission")
}

func checkServerRoute(t *testing.T, addr, path string, expected int) {
	t.Helper()
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get("http://" + addr + path)
	if err != nil {
		t.Fatalf("GET %s%s: %v", addr, path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != expected {
		t.Fatalf("GET %s%s: status %d, want %d", addr, path, resp.StatusCode, expected)
	}
}

func TestServerConcurrentStartAndRestart(t *testing.T) {
	serverTestSetup(t)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); StartServer() }()
	}
	wg.Wait()
	if !Server_Status || len(Server_Listeners) != 2 {
		t.Fatalf("status=%t listeners=%d, want two listeners", Server_Status, len(Server_Listeners))
	}
	for _, listener := range Server_Listeners {
		checkServerRoute(t, listener.Addr().String(), "/ipfilter.dat", http.StatusOK)
		checkServerRoute(t, listener.Addr().String(), "/api/v1/status", http.StatusOK)
	}
	StopServer()
	StartServer()
	if !Server_Status || len(Server_Listeners) != 2 {
		t.Fatalf("restart status=%t listeners=%d", Server_Status, len(Server_Listeners))
	}
	checkServerRoute(t, Server_Listeners[0].Addr().String(), "/ipfilter.dat", http.StatusOK)
}

func TestServerSharedAndOccupiedAddresses(t *testing.T) {
	serverTestSetup(t)
	cfg := *ConfigSnapshot()
	cfg.WebUIListen = cfg.Listen
	ReplaceConfig(&cfg)
	StartServer()
	if len(Server_Listeners) != 1 {
		t.Fatalf("shared address: got %d listeners, want 1", len(Server_Listeners))
	}
	checkServerRoute(t, Server_Listeners[0].Addr().String(), "/api/v1/status", http.StatusOK)
	StopServer()

	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	cfg.Listen = occupied.Addr().String()
	cfg.WebUIListen = "127.0.0.1:0"
	ReplaceConfig(&cfg)
	StartServer()
	if !Server_Status || len(Server_Listeners) != 1 {
		t.Fatalf("partial failure: status=%t listeners=%d", Server_Status, len(Server_Listeners))
	}
	checkServerRoute(t, Server_Listeners[0].Addr().String(), "/api/v1/status", http.StatusOK)
	StopServer()
	cfg.WebUI = false
	ReplaceConfig(&cfg)
	StartServer()
	if Server_Status || len(Server_Listeners) != 0 {
		t.Fatal("server started despite occupied address")
	}
}

func TestServerConcurrentStartStop(t *testing.T) {
	serverTestSetup(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); StartServer() }()
		go func() { defer wg.Done(); StopServer() }()
	}
	wg.Wait()
	StopServer()
	StartServer()
	if !Server_Status || len(Server_Listeners) != 2 {
		t.Fatalf("final start: status=%t listeners=%d", Server_Status, len(Server_Listeners))
	}
	checkServerRoute(t, Server_Listeners[0].Addr().String(), "/ipfilter.dat", http.StatusOK)
}
