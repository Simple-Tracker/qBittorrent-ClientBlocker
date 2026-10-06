package bitcomet

import (
	"net/http"
	"testing"
)

func TestBitCometLegacyHTMLWorkflow(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api_v2/task_list/get":
			http.NotFound(w, r)
		case "/panel/":
			w.Header().Set("WWW-Authenticate", `Basic realm="BitComet"`)
			w.WriteHeader(http.StatusUnauthorized)
		case "/panel/task_list":
			_, _ = w.Write([]byte(`<table><tbody><tr><th>head</th></tr><tr><td>BT</td><td><a href="/panel/task_detail?id=42">x</a></td><td>running</td><td></td><td>1 MB</td><td></td><td></td><td>2 KB/s</td></tr><tr><td>HTTP</td></tr></tbody></table>`))
		case "/panel/task_detail":
			_, _ = w.Write([]byte(`<table><tbody><tr><th>head</th></tr><tr><td>192.0.2.8:6881</td><td>25%</td><td>1 KB/s</td><td>2 KB/s</td><td>3 MB</td><td>4 MB</td><td></td><td></td><td></td><td>LegacyPeer</td></tr><tr><td>myself</td></tr></tbody></table>`))
		default:
			http.NotFound(w, r)
		}
	}))

	if !c.Detect() || c.Version != 1 {
		t.Fatalf("legacy BitComet detection version=%d", c.Version)
	}
	torrents, err := c.FetchTorrents()
	if err != nil || len(torrents) != 1 || torrents[0].Hash != "42" {
		t.Fatalf("legacy torrents=%#v err=%v", torrents, err)
	}
	peers, err := c.FetchTorrentPeers(torrents[0])
	if err != nil || len(peers) != 1 || peers[0].Client != "LegacyPeer" {
		t.Fatalf("legacy peers=%#v err=%v", peers, err)
	}
	if c.ConfigPath() != "" || c.SetURL() || c.SubmitBlockPeer(nil) || c.SubmitShadowBanPeer(nil) {
		t.Fatal("legacy unsupported operations returned success")
	}
}
