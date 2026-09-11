package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mu/internal/auth"
)

func TestCookieWritesRequireTokenOrFirstPartyOrigin(t *testing.T) {
	const owner = "browser_write_owner"
	if err := auth.Create(&auth.Account{ID: owner, Name: owner, Secret: "test"}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		origin, site, token string
		want                bool
	}{
		{"", "", "", false},
		{"null", "cross-site", "", false},
		{"https://attacker.test", "cross-site", "", false},
		{"https://micro.test.attacker.test", "", "", false},
		{"http://micro.test", "", "", false},
		{"https://micro.test", "same-origin", "", true},
		{"", "same-origin", "", true},
		{"", "same-site", "", false},
		{"https://attacker.test", "cross-site", "valid", true},
	} {
		for _, path := range []string{"/mcp", "/agent", "/apps/demo/sdk/service", "/mail", "/anything/webhook"} {
			r := httptest.NewRequest("POST", "https://micro.test"+path, nil)
			r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			// An arbitrary header cannot exempt ambient cookie authentication.
			r.Header.Set("Authorization", "Bearer invalid")
			if tc.token == "valid" {
				r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
			}
			if got := browserWriteAllowed(r); got != tc.want {
				t.Errorf("%s %+v: got %v", path, tc, got)
			}
		}
	}
	if !browserWriteAllowed(httptest.NewRequest("POST", "/mcp", nil)) {
		t.Fatal("non-cookie client blocked")
	}
}
