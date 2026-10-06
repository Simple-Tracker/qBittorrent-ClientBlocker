package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloseLogFileWithNilHandle(t *testing.T) {
	l := NewLogger(Services{})
	if !l.CloseLogFile() {
		t.Fatal("CloseLogFile should succeed for nil handle")
	}
}

func TestLoadLogFirstOpenAndReopen(t *testing.T) {
	settings := Settings{LogToFile: true, LogPath: t.TempDir()}
	l := NewLogger(Services{Settings: func() Settings { return settings }})
	t.Cleanup(func() { l.CloseLogFile() })
	if !l.LoadLog() || l.logFile == nil {
		t.Fatal("LoadLog should populate logFile on first open")
	}
	nextLogPath := filepath.Join(settings.LogPath, "next")
	if err := os.MkdirAll(nextLogPath, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	settings.LogPath = nextLogPath
	if !l.LoadLog() || l.logFile == nil {
		t.Fatal("LoadLog should keep logFile populated after reopen")
	}
	if got := filepath.Dir(l.logFile.Name()); got != nextLogPath {
		t.Fatalf("log file directory=%q want %q", got, nextLogPath)
	}
}

func TestLoggingErrorAndBufferBranches(t *testing.T) {
	testConfig := Settings{}
	l := NewLogger(Services{Settings: func() Settings { return testConfig }})
	t.Cleanup(func() { l.CloseLogFile() })
	directory := t.TempDir()
	closedFile, err := os.Create(filepath.Join(directory, "closed.log"))
	if err != nil {
		t.Fatal(err)
	}
	if err := closedFile.Close(); err != nil {
		t.Fatal(err)
	}
	l.logFile = closedFile
	if l.CloseLogFile() {
		t.Fatal("closing an already closed log file should fail")
	}
	l.logFile = nil

	testConfig.Debug = true
	testConfig.LogDebug = true
	testConfig.LogToFile = true
	testConfig.WebUI = true
	testConfig.LogPath = filepath.Join(directory, "logs")
	l.todayStr = ""
	l.lastLogPath = ""
	if !l.LoadLog() {
		t.Fatal("debug log file did not open")
	}
	l.logBuffer = nil
	l.logBufferMaxSize = 2
	l.Log("Debug-Coverage", "first", false)
	l.Log("Coverage", "second", false)
	l.Log("Coverage", "third", false)
	if len(l.logBuffer) != 2 || !strings.Contains(l.logBuffer[1], "third") {
		t.Fatalf("trimmed WebUI log buffer=%#v", l.logBuffer)
	}

	testConfig.LogToFile = false
	if l.LoadLog() {
		t.Fatal("disabled file logging unexpectedly loaded")
	}
	blockingFile := filepath.Join(directory, "blocking-file")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	testConfig.LogToFile = true
	testConfig.LogPath = filepath.Join(blockingFile, "child")
	if l.LoadLog() {
		t.Fatal("invalid log directory unexpectedly loaded")
	}

	openFailurePath := filepath.Join(directory, "open-failure")
	logFilename := filepath.Join(openFailurePath, dateTime(false)+".txt")
	if err := os.MkdirAll(logFilename, 0o700); err != nil {
		t.Fatal(err)
	}
	testConfig.LogPath = openFailurePath
	l.todayStr = ""
	l.lastLogPath = ""
	if l.LoadLog() {
		t.Fatal("log path ending in a directory unexpectedly opened")
	}
}
