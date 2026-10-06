package main

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestBitCometSubmitsGlobalStatisticsBanToObservedTasks(t *testing.T) {
	installCIDRTest(t, "/24", "/128")
	currentClientType = "BitComet"
	var submissions []BC_v2_BanParams
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/task/peers/ban_ip" {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		var params BC_v2_BanParams
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		submissions = append(submissions, params)
		w.Write([]byte(`{"result":"success"}`))
	}))
	const ip = "203.0.113.10"
	AddIPInfo(ParseIPCIDRByConfig(ip), ip, 6881, "task-a", 0, 1)
	AddIPInfo(ParseIPCIDRByConfig(ip), ip, 6882, "task-b", 0, 1)
	if count := CheckAllIP(ipMap, lastIPMap); count != 1 {
		t.Fatalf("statistical bans=%d, want 1", count)
	}
	if blockPeerMap[ip].InfoHash != "" {
		t.Fatal("global statistics ban unexpectedly has a single task")
	}
	client := &BCClient{Version: 2}
	if !client.SubmitBlockPeer(blockPeerMap) {
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

	// A later task-specific observation must not submit the same IP twice.
	peer := blockPeerMap[ip]
	peer.InfoHash = "task-a"
	blockPeerMap[ip] = peer
	submissions = nil
	if !client.SubmitBlockPeer(blockPeerMap) {
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
	var submissions []BC_v2_BanParams
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params BC_v2_BanParams
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		submissions = append(submissions, params)
		w.Write([]byte(`{"result":"success"}`))
	}))
	const ip = "203.0.113.10"
	AddIPInfo(nil, ip, 6881, "task-a", 0, 1)
	if len(ipMap) != 0 {
		t.Fatal("test requires statistics to be disabled")
	}
	client := &BCClient{Version: 2}
	if !client.SubmitBlockPeer(map[string]BlockPeerInfoStruct{ip: {InfoHash: "task-a"}}) {
		t.Fatal("task-specific ban failed with statistics disabled")
	}
	if len(submissions) != 1 || submissions[0].TaskID != "task-a" || !reflect.DeepEqual(submissions[0].IPList, []string{ip}) {
		t.Fatalf("unexpected submissions: %+v", submissions)
	}
}

func TestBitCometExpiresOnlyOwnedBansAfterLogin(t *testing.T) {
	installCIDRTest(t, "/32", "/128")
	var removals []BC_v2_UnbanParams
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/task/peers/ban_ip", "/panel/":
		case "/api/task/peers/unban_peers":
			var params BC_v2_UnbanParams
			if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
				t.Error(err)
			}
			removals = append(removals, params)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		w.Write([]byte(`{"result":"success"}`))
	}))
	client := &BCClient{Version: 2}
	if !client.SubmitBlockPeer(blockPeerMap) || len(removals) != 0 {
		t.Fatal("empty initial state must not remove existing client bans")
	}
	const ip = "203.0.113.10"
	AddBlockPeer("test", "test", ip, 6881, "task-a", "", "", 0, 0)
	if !client.SubmitBlockPeer(blockPeerMap) || !client.Login() {
		t.Fatal("initial ban or subsequent login failed")
	}
	currentTimestamp += int64(ConfigSnapshot().BanTime) + 1
	if removed := ClearBlockPeer(); removed != 1 {
		t.Fatalf("expired bans=%d, want 1", removed)
	}
	if !client.SubmitBlockPeer(blockPeerMap) {
		t.Fatal("expired ban removal failed")
	}
	want := []BC_v2_UnbanParams{{TaskID: "task-a", UnbanRange: "unban_peers", IPList: []string{ip}}}
	if !reflect.DeepEqual(removals, want) {
		t.Fatalf("unban requests=%+v, want %+v", removals, want)
	}
}

func TestBitCometRetriesOnlyFailedBanAndUnbanTasks(t *testing.T) {
	InstallHistoryTest(t)
	banRequests, unbanRequests := make(map[string]int), make(map[string]int)
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params BC_v2_UnbanParams
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/api/task/peers/ban_ip":
			banRequests[params.TaskID]++
			if params.TaskID == "task-b" && banRequests[params.TaskID] == 1 {
				w.Write([]byte(`{"result":"failed"}`))
				return
			}
		case "/api/task/peers/unban_peers":
			if params.UnbanRange != "unban_peers" {
				t.Errorf("unexpected unban scope %+v", params)
			}
			unbanRequests[params.TaskID]++
			if params.TaskID == "task-b" && unbanRequests[params.TaskID] == 1 {
				w.Write([]byte(`{"error_code":"FAILED"}`))
				return
			}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		w.Write([]byte(`{"error_code":"OK"}`))
	}))
	client := &BCClient{Version: 2}
	peers := map[string]BlockPeerInfoStruct{
		"203.0.113.10": {InfoHash: "task-a"},
		"203.0.113.20": {InfoHash: "task-b"},
		"203.0.113.30": {InfoHash: "task-c"},
	}
	if client.SubmitBlockPeer(peers) {
		t.Fatal("business-level ban failure was reported as success")
	}
	if !client.SubmitBlockPeer(peers) {
		t.Fatal("ban retry failed")
	}
	if want := map[string]int{"task-a": 1, "task-b": 2, "task-c": 1}; !reflect.DeepEqual(banRequests, want) {
		t.Fatalf("ban attempts=%v, want %v", banRequests, want)
	}
	delete(peers, "203.0.113.10")
	delete(peers, "203.0.113.20")
	if client.SubmitBlockPeer(peers) {
		t.Fatal("business-level unban failure was reported as success")
	}
	if !client.SubmitBlockPeer(peers) || !client.SubmitBlockPeer(peers) {
		t.Fatal("unban retry failed")
	}
	if want := map[string]int{"task-a": 1, "task-b": 2}; !reflect.DeepEqual(unbanRequests, want) {
		t.Fatalf("unban attempts=%v, want %v", unbanRequests, want)
	}
	if banRequests["task-c"] != 1 || len(client.bannedTaskIPs) != 1 || !client.bannedTaskIPs["task-c"]["203.0.113.30"] {
		t.Fatal("retained ban was changed")
	}
}

func TestBitCometDoesNotOwnFailedBans(t *testing.T) {
	for _, body := range []string{`{"result":"failed"}`, `{"error_code":"INVALID_REQUEST"}`, `{}`, `{`, `{"result":"success","error_code":"FAILED"}`} {
		t.Run(body, func(t *testing.T) {
			InstallHistoryTest(t)
			requests := 0
			InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/api/task/peers/ban_ip" {
					t.Errorf("failed ban must not be unbanned: %s", r.URL.Path)
				}
				w.Write([]byte(body))
			}))
			client := &BCClient{Version: 2}
			if client.SubmitBlockPeer(map[string]BlockPeerInfoStruct{"203.0.113.10": {InfoHash: "task-a"}}) {
				t.Fatal("failed response was accepted")
			}
			if !client.SubmitBlockPeer(map[string]BlockPeerInfoStruct{}) || requests != 1 {
				t.Fatalf("failed ban was retained as owned: requests=%d", requests)
			}
		})
	}
}

func TestBitCometDoesNotUnbanPreviousClientURL(t *testing.T) {
	InstallHistoryTest(t)
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":"success"}`))
	}))
	client := &BCClient{Version: 2}
	if !client.SubmitBlockPeer(map[string]BlockPeerInfoStruct{"203.0.113.10": {InfoHash: "task-a"}}) {
		t.Fatal("initial ban failed")
	}
	requests := 0
	InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte(`{"result":"success"}`))
	}))
	if !client.SubmitBlockPeer(map[string]BlockPeerInfoStruct{}) || requests != 0 {
		t.Fatal("ownership from another client URL caused an unban request")
	}
}
