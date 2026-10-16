package origin

import (
	"net/http/httptest"
	"testing"
)

func TestX402PublicOriginBehindProxy(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.test")
	for _, tc := range []struct{ configured, want string }{{"m3o.test", "https://m3o.test"}, {"https://m3o.test", "https://m3o.test"}, {"http://m3o.test", "http://m3o.test"}} {
		t.Setenv("X402_HOST", tc.configured)
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080/login", nil)
		r.Header.Set("X-Forwarded-Host", "m3o.test")
		if !IsX402Host(r) || URL(r) != tc.want {
			t.Fatalf("%s: %s", tc.configured, URL(r))
		}
	}
}

func TestSameOriginBehindProxy(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.test")
	for _, tc := range []struct {
		origin, host, forwarded string
		want                    bool
	}{
		{"https://micro.test", "127.0.0.1:8080", "", true},
		{"https://micro.test", "micro.test", "", true},
		{"https://evil.test", "127.0.0.1:8080", "evil.test", false},
		{"http://micro.test", "127.0.0.1:8080", "", false},
		{"https://micro.test/path", "127.0.0.1:8080", "", false},
		{"", "127.0.0.1:8080", "", false},
	} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Forwarded-Host", tc.forwarded)
		if SameOrigin(r) != tc.want {
			t.Errorf("origin=%q host=%q", tc.origin, tc.host)
		}
	}
}
