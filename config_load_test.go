package main

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestLoadConfigRetriesParseFailure(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(filename, []byte(`{"Interval":`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		lastModMutex.Lock()
		delete(configLastMod, filename)
		lastModMutex.Unlock()
	})

	for attempt := 1; attempt <= 2; attempt++ {
		target := ConfigStruct{}
		if status := LoadConfig(filename, true, &target); status != -4 {
			t.Fatalf("attempt %d status=%d, want -4", attempt, status)
		}
	}
}

func TestLoadConfigIgnoresUnknownFields(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(filename, []byte(`{"Interval":8,"UnusedFutureField":"ignored"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		lastModMutex.Lock()
		delete(configLastMod, filename)
		lastModMutex.Unlock()
	})

	target := ConfigStruct{}
	if status := LoadConfig(filename, true, &target); status != 0 || target.Interval != 8 {
		t.Fatalf("load status=%d interval=%d", status, target.Interval)
	}
}
func TestConfigSnapshotConcurrentUpdates(t *testing.T) {
	oldConfig := configSnapshot()
	initialConfig := *oldConfig
	initialConfig.Interval = 0
	initialConfig.ClientURL = "http://client/0"
	replaceConfig(&initialConfig)
	t.Cleanup(func() { replaceConfig(oldConfig) })

	const updates = 1000
	var readers sync.WaitGroup
	stop := make(chan struct{})
	failed := make(chan string, 1)
	for readerIndex := 0; readerIndex < 4; readerIndex++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					currentConfig := configSnapshot()
					expectedURL := "http://client/" + strconv.FormatUint(uint64(currentConfig.Interval), 10)
					if currentConfig.ClientURL != expectedURL {
						select {
						case failed <- currentConfig.ClientURL + " != " + expectedURL:
						default:
						}
						return
					}
				}
			}
		}()
	}

	for index := uint32(0); index < updates; index++ {
		updateConfig(func(newConfig *ConfigStruct) {
			newConfig.Interval = index
			newConfig.ClientURL = "http://client/" + strconv.FormatUint(uint64(index), 10)
		})
	}
	close(stop)
	readers.Wait()
	select {
	case message := <-failed:
		t.Fatal(message)
	default:
	}
}
