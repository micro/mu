package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The OAuth discovery documents are how an MCP client finds out how to
// authenticate. Advertising localhost:8081 as the issuer pointed every client
// at a host it cannot reach — so the standard, mainstream way of connecting to
// this server was broken while the niche one worked.
func TestOAuthIssuerIsThePublicOrigin(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil)
	r.Host = "localhost:8081"
	OAuthMetadataHandler(rec, r)

	body := rec.Body.String()
	if strings.Contains(body, "localhost") {
		t.Fatalf("the discovery document still advertises localhost:\n%s", body)
	}
	for _, want := range []string{
		`"issuer":"https://micro.mu"`,
		`"authorization_endpoint":"https://micro.mu/oauth/authorize"`,
		`"registration_endpoint":"https://micro.mu/oauth/register"`,
	} {
		if !strings.Contains(strings.ReplaceAll(body, " ", ""), strings.ReplaceAll(want, " ", "")) {
			t.Errorf("missing %s in:\n%s", want, body)
		}
	}
}

func TestOAuthProtectedResourceIsThePublicOrigin(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/.well-known/oauth-protected-resource", nil)
	r.Host = "localhost:8081"
	OAuthResourceHandler(rec, r)

	if strings.Contains(rec.Body.String(), "localhost") {
		t.Fatalf("the resource document still advertises localhost:\n%s", rec.Body.String())
	}
}
