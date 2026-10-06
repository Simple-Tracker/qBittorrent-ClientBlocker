package bitcomet

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client"
)

func TestBitCometRetriesOnlyFailedBanAndUnbanTasks(t *testing.T) {
	banRequests, unbanRequests := make(map[string]int), make(map[string]int)
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params UnbanParams
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
	c.Version = 2
	peers := map[string]client.BanTarget{
		"203.0.113.10": {TaskIDs: []string{"task-a"}},
		"203.0.113.20": {TaskIDs: []string{"task-b"}},
		"203.0.113.30": {TaskIDs: []string{"task-c"}},
	}
	if c.SubmitBlockPeer(peers) {
		t.Fatal("business-level ban failure was reported as success")
	}
	if !c.SubmitBlockPeer(peers) {
		t.Fatal("ban retry failed")
	}
	if want := map[string]int{"task-a": 1, "task-b": 2, "task-c": 1}; !reflect.DeepEqual(banRequests, want) {
		t.Fatalf("ban attempts=%v, want %v", banRequests, want)
	}
	delete(peers, "203.0.113.10")
	delete(peers, "203.0.113.20")
	if c.SubmitBlockPeer(peers) {
		t.Fatal("business-level unban failure was reported as success")
	}
	if !c.SubmitBlockPeer(peers) || !c.SubmitBlockPeer(peers) {
		t.Fatal("unban retry failed")
	}
	if want := map[string]int{"task-a": 1, "task-b": 2}; !reflect.DeepEqual(unbanRequests, want) {
		t.Fatalf("unban attempts=%v, want %v", unbanRequests, want)
	}
	if banRequests["task-c"] != 1 || len(c.bannedTaskIPs) != 1 || !c.bannedTaskIPs["task-c"]["203.0.113.30"] {
		t.Fatal("retained ban was changed")
	}
}

func TestBitCometDoesNotOwnFailedBans(t *testing.T) {
	for _, body := range []string{`{"result":"failed"}`, `{"error_code":"INVALID_REQUEST"}`, `{}`, `{`, `{"result":"success","error_code":"FAILED"}`} {
		t.Run(body, func(t *testing.T) {
			requests := 0
			c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/api/task/peers/ban_ip" {
					t.Errorf("failed ban must not be unbanned: %s", r.URL.Path)
				}
				w.Write([]byte(body))
			}))
			c.Version = 2
			if c.SubmitBlockPeer(map[string]client.BanTarget{"203.0.113.10": {TaskIDs: []string{"task-a"}}}) {
				t.Fatal("failed response was accepted")
			}
			if !c.SubmitBlockPeer(map[string]client.BanTarget{}) || requests != 1 {
				t.Fatalf("failed ban was retained as owned: requests=%d", requests)
			}
		})
	}
}

func TestBitCometDoesNotUnbanPreviousClientURL(t *testing.T) {
	c, settings := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":"success"}`))
	}))
	c.Version = 2
	if !c.SubmitBlockPeer(map[string]client.BanTarget{"203.0.113.10": {TaskIDs: []string{"task-a"}}}) {
		t.Fatal("initial ban failed")
	}
	requests := 0
	other, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte(`{"result":"success"}`))
	}))
	settings.URL = other.services.Snapshot().URL
	if !c.SubmitBlockPeer(map[string]client.BanTarget{}) || requests != 0 {
		t.Fatal("ownership from another client URL caused an unban request")
	}
}
