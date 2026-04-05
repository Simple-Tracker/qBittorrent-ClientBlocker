package main

import (
	"os"
	"strings"
	"testing"
)

func TestWriteCrashLogAppendsToCrashLog(t *testing.T) {
	oldConfig := config
	defer func() {
		config = oldConfig
	}()

	config = oldConfig
	config.LogPath = t.TempDir()

	WriteCrashLog("test-location", "panic-a", []byte("stack-a\n"))
	WriteCrashLog("test-location", "panic-b", []byte("stack-b\n"))

	content, err := os.ReadFile(CrashLogPath())
	if err != nil {
		t.Fatalf("failed to read crash log: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "panic: panic-a") || !strings.Contains(contentStr, "panic: panic-b") {
		t.Fatalf("crash log should contain both appended panic records: %q", contentStr)
	}
	if !strings.Contains(contentStr, "stack-a") || !strings.Contains(contentStr, "stack-b") {
		t.Fatalf("crash log should contain stack traces: %q", contentStr)
	}
}
