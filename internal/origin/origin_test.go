package origin

import (
	"net/http/httptest"
	"testing"
)

// Two live bugs came from three places each answering "what is my address"
// differently: the x402 challenge advertised localhost:8081 as the resource
// being paid for, and the OAuth discovery documents advertised it as the
// authorization server — which points every MCP client at a host it cannot
// reach, breaking the mainstream way of connecting.
func TestURLIgnoresTheLoopbackHostBehindAProxy(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")

	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Host = "localhost:8081" // what the proxy actually passes through
	if got := URL(r); got != "https://micro.mu" {
		t.Fatalf("URL = %q, want https://micro.mu", got)
	}
}

func TestURLFallsBackThroughTheProxyThenTheHost(t *testing.T) {
	t.Setenv("MU_DOMAIN", "")

	fwd := httptest.NewRequest("GET", "/", nil)
	fwd.Host = "localhost:8081"
	fwd.Header.Set("X-Forwarded-Host", "micro.mu")
	fwd.Header.Set("X-Forwarded-Proto", "https")
	if got := URL(fwd); got != "https://micro.mu" {
		t.Errorf("forwarded host gave %q", got)
	}

	bare := httptest.NewRequest("GET", "/", nil)
	bare.Host = "example.test:9000"
	if got := URL(bare); got != "http://example.test:9000" {
		t.Errorf("bare host gave %q", got)
	}
}

// "localhost" is the documented default for MU_DOMAIN, so it must not be
// treated as a real public domain.
func TestURLIgnoresTheLocalhostDefault(t *testing.T) {
	t.Setenv("MU_DOMAIN", "localhost")
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "localhost:8081"
	r.Header.Set("X-Forwarded-Host", "micro.mu")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := URL(r); got != "https://micro.mu" {
		t.Errorf("MU_DOMAIN=localhost gave %q, want the forwarded host", got)
	}
}

// A scheme on MU_DOMAIN must not end up doubled.
func TestURLTolerateSchemeOnDomain(t *testing.T) {
	for _, d := range []string{"micro.mu", "https://micro.mu", "http://micro.mu", "micro.mu/"} {
		t.Setenv("MU_DOMAIN", d)
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = "localhost:8081"
		if got := URL(r); got != "https://micro.mu" {
			t.Errorf("MU_DOMAIN=%q gave %q", d, got)
		}
	}
}
