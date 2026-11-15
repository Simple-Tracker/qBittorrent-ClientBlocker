package app

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestFetchFailureCountsByURL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("callback fixture uses a shell script")
	}
	oldConfig, oldClient, oldExternal, oldCounts := config, httpClient, httpClientExternal, fetchFailedCounts
	t.Cleanup(func() {
		config, httpClient, httpClientExternal, fetchFailedCounts = oldConfig, oldClient, oldExternal, oldCounts
	})
	fetchFailedCounts = make(map[string]int)
	dir := t.TempDir()
	script, output := filepath.Join(dir, "callback.sh"), filepath.Join(dir, "calls")
	if err := os.WriteFile(script, []byte("#!/bin/sh\noutput=$1\nshift\nprintf '%s\\n' \"$@\" >> \"$output\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := *oldConfig
	cfg.ClientURL = "http://client.invalid/a path|part"
	cfg.FetchFailedThreshold = 2
	cfg.ExecCommand_FetchFailed = fmt.Sprintf("%q %q {clientURL} {URLWithoutQueryParam}", script, output)
	config = &cfg
	transport := coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("success") == "1" {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
		}
		if r.URL.Query().Get("ok") == "1" {
			return &http.Response{StatusCode: 500, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return nil, errors.New("connection refused")
	})
	httpClientExternal = http.Client{Transport: transport}
	httpClient = http.Client{Transport: transport}
	fetch := func(url string) { t.Helper(); Fetch(url, false, false, false, nil) }
	fetch("http://example.invalid/list?a=1#first")
	fetch("http://example.invalid/other?a=1")
	fetch("http://example.invalid/list?success=1") // Success does not reset the count.
	fetch("http://example.invalid/list?ok=1")      // HTTP errors do not count or reset.
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("callback ran before any URL reached its threshold: %v", err)
	}
	fetch("http://example.invalid/list?a=2#second")
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := cfg.ClientURL + "\nhttp://example.invalid/list\n"
	if string(got) != want {
		t.Fatalf("callback arguments = %q, want %q", got, want)
	}
	if fetchFailedCounts["http://example.invalid/other"] != 1 || fetchFailedCounts["http://example.invalid/list"] != 0 {
		t.Fatalf("unexpected counters: %#v", fetchFailedCounts)
	}
	fetch("http://example.invalid/list?a=3")
	fetch("http://example.invalid/list?a=4")
	got, err = os.ReadFile(output)
	if err != nil || string(got) != want+want {
		t.Fatalf("repeated callback = %q, err=%v", got, err)
	}

	// Submit and disabled callbacks must not change the counter.
	Submit("http://example.invalid/other", nil, false, false, nil)
	cfg.FetchFailedThreshold = 0
	fetch("http://example.invalid/other")
	cfg.FetchFailedThreshold = 2
	cfg.ExecCommand_FetchFailed = ""
	fetch("http://example.invalid/other")
	if fetchFailedCounts["http://example.invalid/other"] != 1 {
		t.Fatalf("disabled or Submit changed counters: %#v", fetchFailedCounts)
	}
}

func TestFetchFailureCountsConcurrent(t *testing.T) {
	oldConfig, oldExternal, oldCounts := config, httpClientExternal, fetchFailedCounts
	t.Cleanup(func() { config, httpClientExternal, fetchFailedCounts = oldConfig, oldExternal, oldCounts })
	fetchFailedCounts = make(map[string]int)
	cfg := *oldConfig
	cfg.FetchFailedThreshold = 1000
	cfg.ExecCommand_FetchFailed = "unused"
	config = &cfg
	httpClientExternal = http.Client{Transport: coverageRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network failure")
	})}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			Fetch(fmt.Sprintf("http://example.invalid/list?item=%d", i), false, false, false, nil)
		}(i)
	}
	wg.Wait()
	if got := fetchFailedCounts["http://example.invalid/list"]; got != 40 {
		t.Fatalf("count = %d, want 40", got)
	}
}
