package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetBlockListFromFileContinuesPastUnchangedFile(t *testing.T) {
	oldConfig := config
	oldBlockListFileLastMod := blockListFileLastMod
	defer func() {
		config = oldConfig
		blockListFileLastMod = oldBlockListFileLastMod
		EraseSyncMap(&blockListCompiled)
	}()

	EraseSyncMap(&blockListCompiled)
	blockListFileLastMod = make(map[string]int64)

	dir := t.TempDir()
	firstFile := filepath.Join(dir, "first.txt")
	secondFile := filepath.Join(dir, "second.txt")

	if err := os.WriteFile(firstFile, []byte("OldAgent\n"), 0644); err != nil {
		t.Fatalf("failed to write first file: %v", err)
	}
	if err := os.WriteFile(secondFile, []byte("NewAgent\n"), 0644); err != nil {
		t.Fatalf("failed to write second file: %v", err)
	}

	firstStat, err := os.Stat(firstFile)
	if err != nil {
		t.Fatalf("failed to stat first file: %v", err)
	}
	blockListFileLastMod[firstFile] = firstStat.ModTime().Unix()

	config = oldConfig
	config.BlockListFile = []string{firstFile, secondFile}

	if !SetBlockListFromFile() {
		t.Fatal("SetBlockListFromFile should succeed")
	}
	if _, exists := blockListCompiled.Load("NewAgent"); !exists {
		t.Fatal("second changed file should still be processed")
	}
}

func TestSetIPBlockListFromFileContinuesPastUnchangedFile(t *testing.T) {
	oldConfig := config
	oldIPBlockListFileLastMod := ipBlockListFileLastMod
	defer func() {
		config = oldConfig
		ipBlockListFileLastMod = oldIPBlockListFileLastMod
		EraseSyncMap(&ipBlockListCompiled)
	}()

	EraseSyncMap(&ipBlockListCompiled)
	ipBlockListFileLastMod = make(map[string]int64)

	dir := t.TempDir()
	firstFile := filepath.Join(dir, "first.txt")
	secondFile := filepath.Join(dir, "second.txt")

	if err := os.WriteFile(firstFile, []byte("1.1.1.1\n"), 0644); err != nil {
		t.Fatalf("failed to write first file: %v", err)
	}
	if err := os.WriteFile(secondFile, []byte("2.2.2.2\n"), 0644); err != nil {
		t.Fatalf("failed to write second file: %v", err)
	}

	firstStat, err := os.Stat(firstFile)
	if err != nil {
		t.Fatalf("failed to stat first file: %v", err)
	}
	ipBlockListFileLastMod[firstFile] = firstStat.ModTime().Unix()

	config = oldConfig
	config.IPBlockListFile = []string{firstFile, secondFile}

	if !SetIPBlockListFromFile() {
		t.Fatal("SetIPBlockListFromFile should succeed")
	}
	if _, exists := ipBlockListCompiled.Load("2.2.2.2"); !exists {
		t.Fatal("second changed IP file should still be processed")
	}
}
