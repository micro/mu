package app

// The Delete control has to send a request a handler recognises, and has to
// notice when it did not work.
//
// It sent a bare POST with no body. Every handler behind it takes a delete as
// POST plus a _method=DELETE field, because a browser form cannot issue a
// DELETE — so on the blog the request fell through to the page renderer and
// came back 200 with the post still there, and on /social the method switch
// reached "case POST" first and created a thread. Then the answer was thrown
// away and the page navigated anyway, so both failures looked exactly like
// success: confirm, redirect, item still there.
//
// Two properties, and the second is the one that would have caught the first:
// send what the handlers take, and believe them when they say no.

import (
	"html"
	"strings"
	"testing"
)

// menuFor is the Delete line as a browser gets it, with the attribute
// unescaped so the script can be read.
func menuFor(t *testing.T, deleteURL string) string {
	t.Helper()
	out := ItemControls("asim", false, "post", "p1", "asim", "", deleteURL)
	if !strings.Contains(out, ">Delete<") {
		t.Fatalf("no Delete control was rendered at all:\n%s", out)
	}
	return html.UnescapeString(out)
}

func TestDeletingSendsWhatTheHandlersTake(t *testing.T) {
	js := menuFor(t, "/blog/post?id=p1")

	if !strings.Contains(js, "_method=DELETE") {
		t.Errorf("Delete sends no _method, so every handler reads it as something "+
			"else — on the blog a page view, on /social a new thread:\n%s", js)
	}
	if !strings.Contains(js, "application/x-www-form-urlencoded") {
		t.Errorf("the body is sent without a form content type, so FormValue "+
			"cannot see the field:\n%s", js)
	}
	// And still as its own credentialled request with the token, which is how
	// it got past CSRF before.
	if !strings.Contains(js, "X-CSRF-Token") || !strings.Contains(js, "same-origin") {
		t.Errorf("the delete no longer carries the session or the token:\n%s", js)
	}
}

// A refusal is shown, not navigated past.
//
// This is the property that made the original bug invisible for as long as it
// was: the page moved on whatever the server said, so "you cannot delete that"
// and "deleted" were the same experience.
func TestARefusedDeleteSaysSo(t *testing.T) {
	js := menuFor(t, "/blog/post?id=p1")

	if !strings.Contains(js, "r.ok") {
		t.Errorf("the response is never checked, so a refusal looks like a "+
			"delete:\n%s", js)
	}
	if !strings.Contains(js, "alert(") {
		t.Errorf("nothing tells the person it failed:\n%s", js)
	}
	// The navigation has to be inside the success path. A window.location that
	// runs before the status is known is the old behaviour wearing a check.
	ok := strings.Index(js, "r.ok")
	nav := strings.Index(js, "window.location=")
	if ok < 0 || nav < 0 || nav < ok {
		t.Errorf("the page navigates before the answer is checked:\n%s", js)
	}
}

// And it lands somewhere that still exists. The item that was on this page is
// gone, so "back" cannot mean the page it was on.
func TestDeletingLandsOnTheListing(t *testing.T) {
	for url, want := range map[string]string{
		"/blog/post?id=p1": "'/blog'",
		"/social?id=p1":    "'/social'",
		"/apps/thing":      "'/apps'",
	} {
		if js := menuFor(t, url); !strings.Contains(js, "window.location="+want) {
			t.Errorf("deleting via %s does not return to %s:\n%s", url, want, js)
		}
	}
}
