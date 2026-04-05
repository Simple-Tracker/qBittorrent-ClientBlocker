package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloseLogFileWithNilHandle(t *testing.T) {
	oldLogFile := logFile
	defer func() {
		logFile = oldLogFile
	}()

	logFile = nil
	if !CloseLogFile() {
		t.Fatal("CloseLogFile should succeed for nil handle")
	}
}

func TestLoadLogFirstOpenAndReopen(t *testing.T) {
	oldConfig := *config
	oldLogFile := logFile
	oldTodayStr := todayStr
	oldLastLogPath := lastLogPath
	defer func() {
		CloseLogFile()
		tmpConf := oldConfig
	config = &tmpConf
		logFile = oldLogFile
		todayStr = oldTodayStr
		lastLogPath = oldLastLogPath
	}()

	tmpConf := oldConfig
	config = &tmpConf
	config.LogToFile = true
	config.LogPath = t.TempDir()
	logFile = nil
	todayStr = ""
	lastLogPath = ""

	if !LoadLog() {
		t.Fatal("LoadLog should succeed on first open")
	}
	if logFile == nil {
		t.Fatal("LoadLog should populate logFile")
	}
	nextLogPath := filepath.Join(config.LogPath, "next")
	if err := os.MkdirAll(nextLogPath, os.ModePerm); err != nil {
		t.Fatalf("failed to prepare second log dir: %v", err)
	}
	config.LogPath = nextLogPath

	if !LoadLog() {
		t.Fatal("LoadLog should succeed on reopen")
	}
	if logFile == nil {
		t.Fatal("LoadLog should keep logFile populated after reopen")
	}
	if got := filepath.Dir(logFile.Name()); got != nextLogPath {
		t.Fatalf("log file should be reopened under the new directory: got %q want %q", got, nextLogPath)
	}
}
