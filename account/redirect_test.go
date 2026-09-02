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
	// The fallback is the front door, not the dashboard. It was /home while the
	// front door was a pitch a signed-in account had no use for; / is the same
	// page for everybody now, and landing on the dashboard put a rail and a grid
	// of sixteen cards in front of somebody whose next move is to type a
	// question. See home.Index.
	for in, want := range map[string]string{
		"":                             "/",
		"/oauth/authorize?client_id=x": "/oauth/authorize?client_id=x",
		"/tools":                       "/tools",
		"//evil.example":               "/", // a browser reads this as a host
		"https://evil.example":         "/",
		"http://evil.example":          "/",
		"evil.example":                 "/",
	} {
		r := httptest.NewRequest("GET", "/signup?redirect="+url.QueryEscape(in), nil)
		if got := safeRedirect(r); got != want {
			t.Errorf("safeRedirect(%q) = %q, want %q", in, got, want)
		}
	}
}
