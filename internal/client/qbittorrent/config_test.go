package qbittorrent

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestQBConfigDiscovery(t *testing.T) {
	c, settings := newTestClient(t, http.NotFoundHandler())
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	path := filepath.Join(dir, ".config", "qBittorrent", "qBittorrent.ini")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "WebUI\\Enabled=true\nWebUI\\HTTPS\\Enabled=true\nWebUI\\Address=::1\nWebUI\\Port=8443\nWebUI\\Username=Admin\nignored\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if c.ConfigPath() != path || !c.SetURL() {
		t.Fatal("qB config discovery failed")
	}
	if settings.URL != "https://[::1]:8443/api" || settings.Username != "admin" {
		t.Fatalf("qB discovered URL=%q username=%q", settings.URL, settings.Username)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if len(c.configFile()) != 0 {
		t.Fatal("missing qB config returned content")
	}
}
