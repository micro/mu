package server

import (
	"net/http/httptest"
	"testing"
	"time"

	"mu/internal/auth"
)

func TestScopedCredentialCannotUseAccountDoors(t *testing.T) {
	owner := "scope_door_owner"
	if err := auth.Create(&auth.Account{ID: owner, Name: owner, Secret: "test"}); err != nil {
		t.Fatal(err)
	}
	_, secret, err := auth.CreateToken(owner, "restricted", auth.ScopeFor([]string{"news"}), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"Authorization", "X-Micro-Token"} {
		for _, path := range []string{"/mail", "/agent", "/agent/run", "/account", "/apps/example/sdk/service", "/files/private.pdf", "/mcp/../mail", "/api/v1x/mail/inbox", "/api/v1/../../mail", "/api/v1/%2e%2e/%2e%2e/mail"} {
			r := httptest.NewRequest("POST", path, nil)
			r.Header.Set(header, secret)
			if scopedRequestAllowed(r) {
				t.Errorf("%s reached %s", header, path)
			}
		}
		for _, path := range []string{"/mcp", "/mcp/", "/api/v1", "/api/v1/", "/api/v1/news/list", "/api/v1/news/list/"} {
			r := httptest.NewRequest("POST", path, nil)
			r.Header.Set(header, secret)
			if !scopedRequestAllowed(r) {
				t.Errorf("%s denied %s", header, path)
			}
		}
	}
	if !scopedRequestAllowed(httptest.NewRequest("GET", "/agent", nil)) {
		t.Fatal("browser route blocked")
	}
}
