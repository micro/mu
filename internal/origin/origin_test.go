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

// A second public surface is a request identity, not a second deployment. The
// configured instance domain still names background work, while a request that
// entered through m3o.com must advertise m3o.com in OAuth and x402 responses.
func TestURLPreservesExplicitPublicSurface(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")

	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Host = "localhost:8081"
	r.Header.Set("X-Mu-Surface", "m3o")
	r.Header.Set("X-Forwarded-Host", "m3o.com")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := URL(r); got != "https://m3o.com" {
		t.Fatalf("URL = %q, want https://m3o.com", got)
	}

	// Merely forwarding a different host does not override the configured
	// canonical instance. The proxy has to opt into a second public surface.
	r.Header.Del("X-Mu-Surface")
	if got := URL(r); got != "https://micro.mu" {
		t.Fatalf("URL without surface = %q, want https://micro.mu", got)
	}
}

func TestURLPreservesConfiguredX402Host(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")
	t.Setenv("X402_HOST", "pay.example")

	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Host = "localhost:8081"
	r.Header.Set("X-Forwarded-Host", "pay.example")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := URL(r); got != "https://pay.example" {
		t.Fatalf("URL = %q, want https://pay.example", got)
	}
	if !IsX402Host(r) {
		t.Fatal("IsX402Host = false, want true")
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
