package app

import (
	"sync/atomic"
	"testing"
)

func TestGCUsesConfiguredInterval(t *testing.T) {
	oldConfig := *config
	oldTimestamp := atomic.LoadInt64(&currentTimestamp)
	oldLastRun := atomic.LoadInt64(&lastGCTimestamp)
	oldRuntimeGC := runRuntimeGC
	t.Cleanup(func() {
		restored := oldConfig
		config = &restored
		atomic.StoreInt64(&currentTimestamp, oldTimestamp)
		atomic.StoreInt64(&lastGCTimestamp, oldLastRun)
		runRuntimeGC = oldRuntimeGC
	})

	testConfig := oldConfig
	testConfig.GCInterval = 60
	config = &testConfig
	atomic.StoreInt64(&lastGCTimestamp, 0)
	runs := 0
	runRuntimeGC = func() { runs++ }

	atomic.StoreInt64(&currentTimestamp, 100)
	GC()
	GC()
	if runs != 1 {
		t.Fatalf("runtime GC runs=%d, want 1", runs)
	}

	atomic.StoreInt64(&currentTimestamp, 159)
	GC()
	if runs != 1 {
		t.Fatalf("runtime GC ran before the configured interval: %d", runs)
	}

	atomic.StoreInt64(&currentTimestamp, 160)
	GC()
	if runs != 2 {
		t.Fatalf("runtime GC runs=%d, want 2", runs)
	}

	testConfig.GCInterval = 0
	atomic.StoreInt64(&currentTimestamp, 220)
	GC()
	if runs != 2 {
		t.Fatalf("disabled runtime GC runs=%d, want 2", runs)
	}
}
