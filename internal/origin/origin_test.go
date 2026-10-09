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
