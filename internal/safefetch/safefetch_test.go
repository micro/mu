package safefetch

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", // loopback
		"10.0.0.5", "192.168.1.1", "172.16.0.1", // private
		"169.254.169.254", // link-local / cloud metadata
		"fe80::1",         // link-local v6
		"fc00::1",         // unique-local v6
		"0.0.0.0", "::",   // unspecified
		"224.0.0.1", "ff02::1", // multicast
	}
	for _, s := range blocked {
		if !isBlockedIP(net.ParseIP(s)) {
			t.Errorf("isBlockedIP(%s) = false, want true", s)
		}
	}
	public := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:2800:220:1::1"}
	for _, s := range public {
		if isBlockedIP(net.ParseIP(s)) {
			t.Errorf("isBlockedIP(%s) = true, want false", s)
		}
	}
}

func TestValidateURLRejects(t *testing.T) {
	bad := []string{
		"file:///etc/passwd",
		"ftp://example.com/x",
		"http://127.0.0.1:8080/admin",
		"http://localhost/x",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.1.2.3/internal",
		"http://[::1]/x",
		"not-a-url",
		"http://",
	}
	for _, u := range bad {
		if _, err := validateURL(u); err == nil {
			t.Errorf("validateURL(%q) = nil error, want rejection", u)
		}
	}
}

func TestFetchBlocksLoopback(t *testing.T) {
	// A real loopback server the guard must refuse to reach.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret"))
	}))
	defer srv.Close()
	if _, err := Fetch(context.Background(), srv.URL, Options{}); err == nil {
		t.Fatalf("Fetch(%s) succeeded; loopback must be blocked", srv.URL)
	}
}
