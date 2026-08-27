package blog

// Where a finished post lands.

import "testing"

// The form can send you back where you wrote from — and only to somewhere here.
//
// The compose box on a profile posts to /blog, so without this a post written
// on your own page dropped you on the blog index. With it, /blog is a redirect
// anybody can hand a stranger a link to, which is the other half of the test:
// every way of naming another host has to come out as /blog, or this instance
// forwards people to somewhere else under its own name.
func TestAPostComesBackToWhereItWasWritten(t *testing.T) {
	if got := returnTo("/@asim"); got != "/@asim" {
		t.Errorf("writing from a profile landed at %q", got)
	}
	if got := returnTo("  /inbox  "); got != "/inbox" {
		t.Errorf("a padded path came out %q", got)
	}
	if got := returnTo(""); got != "/blog" {
		t.Errorf("a form that asked for nothing landed at %q rather than the blog", got)
	}

	// Anything that is not a path on this instance.
	for _, elsewhere := range []string{
		"https://example.test/phish",
		"http://example.test",
		"//example.test",      // scheme-relative: a host, not a path
		"///example.test",     // and the same trick with more slashes
		"\\\\example.test",    // backslashes, which some clients normalise
		"javascript:alert(1)", //nolint:gosec // not a location at all
		"/\t/example.test",    // a path only until something strips the tab
		" //example.test",     // leading space, trimmed back to a host
		"@example.test",       //
		"blog",                // relative, resolves against the current path
		"https:/example.test", // one slash, still a scheme
	} {
		if got := returnTo(elsewhere); got != "/blog" {
			t.Errorf("a post could send somebody to %q: landed at %q", elsewhere, got)
		}
	}
}
