package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestWebUIStatusDuringClientTypeChanges(t *testing.T) {
	old := CurrentClientTypeSnapshot()
	t.Cleanup(func() { SetCurrentClientType(old) })
	var done sync.WaitGroup
	done.Add(1)
	go func() {
		defer done.Done()
		for i := 0; i < 500; i++ {
			WebUI_GetStatus(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/status", nil))
		}
	}()
	for i := 0; i < 500; i++ {
		SetCurrentClientType("qBittorrent")
		SetCurrentClientType("Transmission")
	}
	done.Wait()
}
