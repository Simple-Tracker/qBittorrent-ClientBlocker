package main

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestDefaultConfigFilename(t *testing.T) {
	if configFilename != "config.json" {
		t.Fatalf("default config filename=%q, want config.json", configFilename)
	}
}

func TestDefaultConfigFilenameLoadsFromWorkingDirectory(t *testing.T) {
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "config.json"), []byte(`{
		"interval": 7,
		"unknownField": true,
		"blockListFile": [
			"blockList.json",
			// "optional.json"
		]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWorkingDirectory)
		lastModMutex.Lock()
		delete(configLastMod, configFilename)
		lastModMutex.Unlock()
	})

	target := ConfigStruct{}
	if status := LoadConfig(configFilename, true, &target); status != 0 {
		t.Fatalf("load default config status=%d, want 0", status)
	}
	if target.Interval != 7 || len(target.BlockListFile) != 1 || target.BlockListFile[0] != "blockList.json" {
		t.Fatalf("unexpected default config: %#v", target)
	}
}

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
	content := `{
		"interval": 8,
		"logDebug_CheckPeer": false,
		"unusedFutureField": "ignored",
		"blockListFile": [
			"blockList.json",
			//"blockList-Optional.json"
		],
		"ipBlockListURL": [
			"https://example.com/all.txt",
			//"https://example.com/optional.txt",
			"https://example.com/project.txt"
		],
		"webUI": true
	}`
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		lastModMutex.Lock()
		delete(configLastMod, filename)
		lastModMutex.Unlock()
	})

	target := ConfigStruct{}
	if status := LoadConfig(filename, true, &target); status != 0 {
		t.Fatalf("load status=%d, want 0", status)
	}
	if target.Interval != 8 || !target.WebUI || len(target.BlockListFile) != 1 || len(target.IPBlockListURL) != 2 {
		t.Fatalf("unexpected config: %#v", target)
	}
}

func TestLoadConfigTOMLAndStatusCodes(t *testing.T) {
	directory := t.TempDir()
	missing := filepath.Join(directory, "missing.toml")
	target := ConfigStruct{}
	if status := LoadConfig(missing, false, &target); status != -5 {
		t.Fatalf("missing status=%d, want -5", status)
	}

	filename := filepath.Join(directory, "config.toml")
	if err := os.WriteFile(filename, []byte("Interval = 9\nUnusedFutureField = \"ignored\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		lastModMutex.Lock()
		delete(configLastMod, filename)
		lastModMutex.Unlock()
	})
	if status := LoadConfig(filename, true, &target); status != 0 || target.Interval != 9 {
		t.Fatalf("TOML load status=%d interval=%d", status, target.Interval)
	}
	if status := LoadConfig(filename, true, &target); status != -1 {
		t.Fatalf("unchanged status=%d, want -1", status)
	}
}

func TestConfigSnapshotConcurrentUpdates(t *testing.T) {
	oldConfig := ConfigSnapshot()
	initialConfig := *oldConfig
	initialConfig.Interval = 0
	initialConfig.ClientURL = "http://client/0"
	ReplaceConfig(&initialConfig)
	t.Cleanup(func() { ReplaceConfig(oldConfig) })

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
					currentConfig := ConfigSnapshot()
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
		UpdateConfig(func(newConfig *ConfigStruct) {
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

func TestConfigSnapshotRemainsImmutableAfterUpdate(t *testing.T) {
	originalConfig := ConfigSnapshot()
	initialConfig := *originalConfig
	initialConfig.Interval = 6
	initialConfig.ClientURL = "http://client/old"
	ReplaceConfig(&initialConfig)
	t.Cleanup(func() { ReplaceConfig(originalConfig) })

	oldSnapshot := ConfigSnapshot()
	newSnapshot := UpdateConfig(func(newConfig *ConfigStruct) {
		newConfig.Interval = 12
		newConfig.ClientURL = "http://client/new"
	})

	if oldSnapshot.Interval != 6 || oldSnapshot.ClientURL != "http://client/old" {
		t.Fatalf("old snapshot changed: %#v", oldSnapshot)
	}
	if newSnapshot == oldSnapshot {
		t.Fatal("update reused the old config snapshot")
	}
	if currentConfig := ConfigSnapshot(); currentConfig != newSnapshot || currentConfig.Interval != 12 || currentConfig.ClientURL != "http://client/new" {
		t.Fatalf("new snapshot was not published: %#v", currentConfig)
	}
}
