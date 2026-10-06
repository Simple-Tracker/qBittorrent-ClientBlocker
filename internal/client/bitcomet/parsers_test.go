package bitcomet

import "testing"

func TestBitCometParsers(t *testing.T) {
	if got := ParseTorrentLink("/panel/task_detail?id=42&x=1"); got != 42 {
		t.Fatalf("torrent id=%d", got)
	}
	if got := ParseSize("1.5 MB"); got != 1572864 {
		t.Fatalf("size=%d", got)
	}
	if got := ParseSpeed("2 KB/s"); got != 2048 {
		t.Fatalf("speed=%d", got)
	}
	if got := ParsePercent("25%"); got != 25 {
		t.Fatalf("percent=%f", got)
	}
	if ip, port := ParseIP("[2001:db8::1]:6881"); ip != "[2001:db8::1]" || port != 6881 {
		t.Fatalf("IP=%q port=%d", ip, port)
	}
	if ParseTorrentLink("/panel/task_detail") != -2 || ParseTorrentLink("/panel/task_detail?id=bad") != -3 {
		t.Fatal("invalid torrent links were accepted")
	}
	for _, invalid := range []string{"bad", "1 XB", "x MB"} {
		if ParseSize(invalid) >= 0 {
			t.Fatalf("invalid size %q was accepted", invalid)
		}
	}
	if ParseSpeed("1 KB") >= 0 || ParsePercent("bad%") >= 0 {
		t.Fatal("invalid speed or percentage was accepted")
	}
	if ip, port := ParseIP("myself"); ip != "" || port != -1 {
		t.Fatalf("myself IP=%q port=%d", ip, port)
	}
	if ip, port := ParseIP("192.0.2.1:bad"); ip != "" || port != -3 {
		t.Fatalf("invalid port IP=%q port=%d", ip, port)
	}
}
