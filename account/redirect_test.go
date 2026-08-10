package account

// Where someone lands after signing in or signing up. The test moved with the
// pages: safeRedirect is theirs, and an open redirect on a login page is a
// phishing primitive reachable from a link an OAuth client hands to a user.

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

// Where someone lands after signing in or signing up. Same-site only: an open
// redirect on a login page is a phishing primitive, and this one is reachable
// from a link an OAuth client hands to a user.
func TestSafeRedirect(t *testing.T) {
	for in, want := range map[string]string{
		"":                             "/home",
		"/oauth/authorize?client_id=x": "/oauth/authorize?client_id=x",
		"/tools":                       "/tools",
		"//evil.example":               "/home", // a browser reads this as a host
		"https://evil.example":         "/home",
		"http://evil.example":          "/home",
		"evil.example":                 "/home",
	} {
		r := httptest.NewRequest("GET", "/signup?redirect="+url.QueryEscape(in), nil)
		if got := safeRedirect(r); got != want {
			t.Errorf("safeRedirect(%q) = %q, want %q", in, got, want)
		}
	}
}
