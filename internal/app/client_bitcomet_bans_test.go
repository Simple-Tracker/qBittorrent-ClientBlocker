package app

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/bitcomet"
)

func TestBitCometSubmitsGlobalStatisticsBanToObservedTasks(t *testing.T) {
	installCIDRTest(t, "/24", "/128")
	currentClientType = "BitComet"
	var submissions []bitcomet.BanParams
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/task/peers/ban_ip" {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		var params bitcomet.BanParams
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		submissions = append(submissions, params)
		w.Write([]byte(`{"result":"success"}`))
	}))
	const ip = "203.0.113.10"
	statistics.AddIPInfo(ParseIPCIDRByConfig(ip), ip, 6881, "task-a", 0, 1)
	statistics.AddIPInfo(ParseIPCIDRByConfig(ip), ip, 6882, "task-b", 0, 1)
	if count := statistics.CheckAllIP(); count != 1 {
		t.Fatalf("statistical bans=%d, want 1", count)
	}
	if blockPeerMap[ip].InfoHash != "" {
		t.Fatal("global statistics ban unexpectedly has a single task")
	}
	client := bitcomet.New(ClientServices())
	client.Version = 2
	if !client.SubmitBlockPeer(ToClientBans(blockPeerMap)) {
		t.Fatal("global statistics ban submission failed")
	}
	got := make(map[string][]string)
	for _, submission := range submissions {
		got[submission.TaskID] = append(got[submission.TaskID], submission.IPList...)
	}
	want := map[string][]string{"task-a": {ip}, "task-b": {ip}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("submitted task IPs=%v, want %v", got, want)
	}

	// 后续针对特定任务的观测不得重复提交同一 IP.
	peer := blockPeerMap[ip]
	peer.InfoHash = "task-a"
	blockPeerMap[ip] = peer
	submissions = nil
	if !client.SubmitBlockPeer(ToClientBans(blockPeerMap)) {
		t.Fatal("task-specific submission failed")
	}
	for _, submission := range submissions {
		if len(submission.IPList) != 1 || submission.IPList[0] != ip {
			t.Fatalf("duplicate IP in submission: %+v", submission)
		}
	}
}

func TestBitCometTaskSpecificBanWithoutStatistics(t *testing.T) {
	InstallHistoryTest(t)
	UpdateConfig(func(c *ConfigStruct) {
		c.MaxIPPortCount = 0
		c.IPUploadedCheck = false
		c.BanByRelativeProgressUploaded = false
		c.BTNSubmitPeers, c.BTNSubmitHistories = false, false
		c.SyncServerURL = ""
	})
	var submissions []bitcomet.BanParams
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params bitcomet.BanParams
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		submissions = append(submissions, params)
		w.Write([]byte(`{"result":"success"}`))
	}))
	const ip = "203.0.113.10"
	statistics.AddIPInfo(nil, ip, 6881, "task-a", 0, 1)
	if len(statistics.State().IPMap) != 0 {
		t.Fatal("test requires statistics to be disabled")
	}
	client := bitcomet.New(ClientServices())
	client.Version = 2
	if !client.SubmitBlockPeer(ToClientBans(map[string]BlockPeerInfoStruct{ip: {InfoHash: "task-a"}})) {
		t.Fatal("task-specific ban failed with statistics disabled")
	}
	if len(submissions) != 1 || submissions[0].TaskID != "task-a" || !reflect.DeepEqual(submissions[0].IPList, []string{ip}) {
		t.Fatalf("unexpected submissions: %+v", submissions)
	}
}

func TestBitCometExpiresOnlyOwnedBansAfterLogin(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	var removals []bitcomet.UnbanParams
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/task/peers/ban_ip", "/panel/":
		case "/api/task/peers/unban_peers":
			var params bitcomet.UnbanParams
			if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
				t.Error(err)
			}
			removals = append(removals, params)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		w.Write([]byte(`{"result":"success"}`))
	}))
	client := bitcomet.New(ClientServices())
	client.Version = 2
	if !client.SubmitBlockPeer(ToClientBans(blockPeerMap)) || len(removals) != 0 {
		t.Fatal("empty initial state must not remove existing client bans")
	}
	const ip = "203.0.113.10"
	AddBlockPeer("test", "test", ip, 6881, "task-a", "", "", 0, 0)
	if !client.SubmitBlockPeer(ToClientBans(blockPeerMap)) || !client.Login() {
		t.Fatal("initial ban or subsequent login failed")
	}
	currentTimestamp += int64(ConfigSnapshot().BanTime) + 1
	if removed := ClearBlockPeer(); removed != 1 {
		t.Fatalf("expired bans=%d, want 1", removed)
	}
	if !client.SubmitBlockPeer(ToClientBans(blockPeerMap)) {
		t.Fatal("expired ban removal failed")
	}
	want := []bitcomet.UnbanParams{{TaskID: "task-a", UnbanRange: "unban_peers", IPList: []string{ip}}}
	if !reflect.DeepEqual(removals, want) {
		t.Fatalf("unban requests=%+v, want %+v", removals, want)
	}
}
