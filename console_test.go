package main

import (
	"sync"
	"testing"
)

func TestProcessVersion(t *testing.T) {
	testCases := []struct {
		version string
		wantT   int
		wantM   int
		wantS   int
		wantS2  int
		wantRaw string
	}{
		{"5.2", 0, 5, 2, 0, "5.2"},
		{"5.2b3", 1, 5, 2, 3, "5.2b3"},
		{"5.2p1", 0, 5, 2, 1, "5.2p1"},
		{"5.2.0 (Nightly)", -2, 0, 0, 0, ""},
		{"Unknown", -1, 0, 0, 0, ""},
		{"5_2_0", -3, 0, 0, 0, ""},
	}

	for _, tc := range testCases {
		gotT, gotM, gotS, gotS2, gotRaw := ProcessVersion(tc.version)
		if gotT != tc.wantT || gotM != tc.wantM || gotS != tc.wantS || gotS2 != tc.wantS2 || gotRaw != tc.wantRaw {
			t.Fatalf("ProcessVersion(%q) = (%d,%d,%d,%d,%q), want (%d,%d,%d,%d,%q)", tc.version, gotT, gotM, gotS, gotS2, gotRaw, tc.wantT, tc.wantM, tc.wantS, tc.wantS2, tc.wantRaw)
		}
	}
}

func TestReqStopIdempotent(t *testing.T) {
	oldReqStopChan := reqStopChan
	oldReqStopLogged := reqStopLogged.Load()
	reqStopChan = make(chan struct{})
	reqStopOnce = sync.Once{}
	reqStopLogged.Store(false)
	defer func() {
		reqStopChan = oldReqStopChan
		reqStopOnce = sync.Once{}
		reqStopLogged.Store(oldReqStopLogged)
	}()

	ReqStop()
	ReqStop()

	select {
	case <-reqStopChan:
	default:
		t.Fatal("ReqStop should close reqStopChan")
	}

	if !reqStopLogged.Load() {
		t.Fatal("ReqStop should mark stop as logged")
	}
}
