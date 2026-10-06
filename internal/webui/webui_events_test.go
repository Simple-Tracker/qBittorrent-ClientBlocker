package webui

import (
	"strconv"
	"testing"
)

func TestWebUIPeerSyncEventBufferKeepsNewestEventsInOrder(t *testing.T) {
	webUIPeerSyncMutex.Lock()
	oldCursor := webUIPeerSyncCursor
	oldEvents := webUIPeerSyncEvents
	oldEventStart := webUIPeerSyncEventStart
	webUIPeerSyncCursor = 0
	webUIPeerSyncEvents = nil
	webUIPeerSyncEventStart = 0
	webUIPeerSyncMutex.Unlock()
	t.Cleanup(func() {
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncCursor = oldCursor
		webUIPeerSyncEvents = oldEvents
		webUIPeerSyncEventStart = oldEventStart
		webUIPeerSyncMutex.Unlock()
	})

	for index := 0; index < webUIMaxPeerEvents+2; index++ {
		AppendWebUIBlockPeerEvent(webUIBlockPeerEvent{RemovedIP: strconv.Itoa(index)})
	}

	webUIPeerSyncMutex.Lock()
	defer webUIPeerSyncMutex.Unlock()
	if len(webUIPeerSyncEvents) != webUIMaxPeerEvents {
		t.Fatalf("event count=%d, want %d", len(webUIPeerSyncEvents), webUIMaxPeerEvents)
	}
	if first := WebUIBlockPeerEventAt(0); first.Cursor != 3 || first.RemovedIP != "2" {
		t.Fatalf("oldest event=%#v", first)
	}
	if last := WebUIBlockPeerEventAt(len(webUIPeerSyncEvents) - 1); last.Cursor != webUIMaxPeerEvents+2 {
		t.Fatalf("newest event=%#v", last)
	}
}

func TestWebUIPeerSyncResetsAfterRestart(t *testing.T) {
	webUIPeerSyncMutex.Lock()
	oldEpoch, oldCursor, oldEvents, oldStart := webUIPeerSyncEpoch, webUIPeerSyncCursor, webUIPeerSyncEvents, webUIPeerSyncEventStart
	webUIPeerSyncEpoch, webUIPeerSyncCursor, webUIPeerSyncEvents = "new-instance", 0, nil
	webUIPeerSyncMutex.Unlock()
	t.Cleanup(func() {
		webUIPeerSyncMutex.Lock()
		webUIPeerSyncEpoch, webUIPeerSyncCursor, webUIPeerSyncEvents, webUIPeerSyncEventStart = oldEpoch, oldCursor, oldEvents, oldStart
		webUIPeerSyncMutex.Unlock()
	})
	response := GetWebUIBlockPeerSync("0", "old-instance")
	if !response.Reset || response.Epoch != "new-instance" {
		t.Fatalf("old instance cursor accepted: %#v", response)
	}
	response = GetWebUIBlockPeerSync("0", "new-instance")
	if response.Reset || len(response.Peers) != 0 {
		t.Fatal("unchanged current instance did not return empty delta")
	}
}
