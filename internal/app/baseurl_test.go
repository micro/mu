package app

import (
	"net/http/httptest"
	"testing"
)

// The live x402 challenge advertised https://localhost:8081/mcp as the resource
// being paid for, because it was built from r.Host and Mu sits behind a proxy
// that forwards to a loopback port. A client checks that field against what it
// is calling, so it has to be the public origin.
func TestBaseURLIgnoresLoopbackHostBehindProxy(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")

	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Host = "localhost:8081" // what the proxy actually passes through
	if got := BaseURL(r); got != "https://micro.mu" {
		t.Fatalf("BaseURL = %q, want https://micro.mu", got)
	}
}

// A scheme on MU_DOMAIN must not end up doubled.
func TestBaseURLTolerationsOnDomain(t *testing.T) {
	for _, domain := range []string{"micro.mu", "https://micro.mu", "http://micro.mu"} {
		t.Setenv("MU_DOMAIN", domain)
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = "localhost:8081"
		if got := BaseURL(r); got != "https://micro.mu" {
			t.Errorf("MU_DOMAIN=%q gave %q, want https://micro.mu", domain, got)
		}
	}
}

// Without MU_DOMAIN, fall back to what the proxy reports, then to r.Host.
func TestBaseURLFallsBackToForwardedHost(t *testing.T) {
	t.Setenv("MU_DOMAIN", "")

	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "localhost:8081"
	r.Header.Set("X-Forwarded-Host", "micro.mu")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := BaseURL(r); got != "https://micro.mu" {
		t.Errorf("forwarded host gave %q, want https://micro.mu", got)
	}

	plain := httptest.NewRequest("GET", "/", nil)
	plain.Host = "example.test:9000"
	if got := BaseURL(plain); got != "http://example.test:9000" {
		t.Errorf("bare host gave %q, want http://example.test:9000", got)
	}
}

// "localhost" is the documented default for MU_DOMAIN, so it must not be
// treated as a real public domain.
func TestBaseURLIgnoresLocalhostDomainDefault(t *testing.T) {
	t.Setenv("MU_DOMAIN", "localhost")
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "localhost:8081"
	r.Header.Set("X-Forwarded-Host", "micro.mu")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := BaseURL(r); got != "https://micro.mu" {
		t.Errorf("MU_DOMAIN=localhost gave %q, want the forwarded host", got)
	}
}
