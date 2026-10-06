package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestAndScanDelayCanBeCancelled(t *testing.T) {
	oldContext, oldClient := requestContext, httpClient
	ctx, cancel := context.WithCancel(context.Background())
	requestContext = ctx
	t.Cleanup(func() { cancel(); requestContext, httpClient = oldContext, oldClient })
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	httpClient = *server.Client()
	done := make(chan struct{})
	go func() { defer close(done); Fetch(server.URL, false, true, false, nil) }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not cancel")
	}
	if WaitRequestDelay(time.Minute) {
		t.Fatal("cancelled scan delay completed normally")
	}
}
