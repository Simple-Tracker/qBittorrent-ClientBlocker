package app

import (
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/Simple-Tracker/qBittorrent-ClientBlocker/internal/client/qbittorrent"
)

func TestCIDRBansSubmitValidQBEndpoints(t *testing.T) {
	for _, mode := range []string{"banPeers", "shadowBan"} {
		t.Run(mode, func(t *testing.T) {
			installCIDRTest(t, "/24", "/60")
			var addresses []string
			InstallClientTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				for _, endpoint := range strings.Split(r.Form.Get("peers"), "|") {
					ip, _, err := net.SplitHostPort(endpoint)
					if err != nil {
						t.Errorf("invalid endpoint %q: %v", endpoint, err)
					}
					addresses = append(addresses, ip)
				}
				w.Write([]byte("Ok."))
			}))
			// 使用端口规则验证 ProcessPeer 和配置中的两种子网掩码.
			UpdateConfig(func(c *ConfigStruct) { c.PortBlockList = []uint32{6881} })
			ips := []string{"203.0.113.10", "2001:db8:1234:7a51::10"}
			for _, ip := range ips {
				if processCIDRTestPeer(ip, 6881, .5, 1<<20) != 1 {
					t.Fatal("port rule did not ban the peer")
				}
			}
			client := qbittorrent.New(ClientServices())
			qB_useNewBanPeersMethod = true
			if mode == "shadowBan" {
				if !client.SubmitShadowBanPeer(ToClientBans(blockPeerMap)) {
					t.Fatal("shadow ban failed")
				}
				ips = expectedCIDRBanIPs(ips...)
			} else if !client.SubmitBlockPeer(ToClientBans(blockPeerMap)) {
				t.Fatal("endpoint ban failed")
			}
			assertCIDRTestIPSet(t, addresses, ips)
		})
	}
}
