package account

// Where you come back to after signing in.

import "testing"

// And the destination is a page here, whoever wrote the link.
//
// The value arrives on a URL anybody can compose and ends up in a Location
// header, so a link to our own login page could otherwise land somebody on
// somebody else's site with our domain in the address bar on the way — which
// is what makes a phishing page convincing.
func TestWhereYouComeBackToIsAPageHere(t *testing.T) {
	for _, elsewhere := range []string{
		"//evil.example",       // protocol-relative
		"/\\evil.example",      // a backslash a browser folds into a slash
		"/\tevil.example",      // stripped by the parser before it is followed
		"https://evil.example", // not even a path
		"evil.example",
	} {
		if got := SafeRedirectTo(elsewhere); got != "/" {
			t.Errorf("SafeRedirectTo(%q) = %q — that leaves this instance", elsewhere, got)
		}
	}
	// And a real page still works.
	if got := SafeRedirectTo("/archive?q=x"); got != "/archive?q=x" {
		t.Errorf("SafeRedirectTo dropped a legitimate destination: %q", got)
	}
	// A login page is not a destination — it is a loop.
	for _, loop := range []string{"/login", "/logout", "/signup?invite=x"} {
		if got := SafeRedirectTo(loop); got != "/" {
			t.Errorf("SafeRedirectTo(%q) = %q", loop, got)
		}
	}
}
