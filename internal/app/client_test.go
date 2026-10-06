package app

import (
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/qbittorrent"
	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/transmission"
)

func TestIsSupportClient(t *testing.T) {
	oldClient := currentClient
	defer func() { currentClient = oldClient }()

	currentClient = nil
	if IsSupportClient() {
		t.Fatalf("IsSupportClient() should be false when currentClient is nil")
	}

	currentClient = qbittorrent.New(ClientServices())
	if !IsSupportClient() {
		t.Fatalf("IsSupportClient() should be true when currentClient exists")
	}
}

func TestIsBanPort(t *testing.T) {
	oldClientType, oldClient := currentClientType, currentClient
	oldFlag := qB_useNewBanPeersMethod
	defer func() {
		currentClientType, currentClient = oldClientType, oldClient
		qB_useNewBanPeersMethod = oldFlag
	}()

	currentClientType = "qBittorrent"
	currentClient = qbittorrent.New(ClientServices())
	qB_useNewBanPeersMethod = true
	if !IsBanPort() {
		t.Fatalf("IsBanPort() should be true for qBittorrent with new ban method")
	}

	currentClientType = "Transmission"
	currentClient = transmission.New(ClientServices())
	if IsBanPort() {
		t.Fatalf("IsBanPort() should be false for non-qBittorrent client")
	}
}

func TestSubmitBlockPeer_NilInput(t *testing.T) {
	if !SubmitBlockPeer(nil) {
		t.Fatalf("SubmitBlockPeer(nil) should return true")
	}
}
