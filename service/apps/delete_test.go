package apps

// Deleting is a write, so the control has to post.
//
// The Delete on the apps list was a plain <a href="/apps/<slug>/delete"> with a
// confirm on it. The route is POST-only, so the click was a GET, matched
// nothing and 404d — after the dialog had already asked whether you were sure.
// The same control on the app's own page did it by fetch and worked, so it
// depended which page you happened to be looking at.
//
// The rule is not "the route insists". A link is something a browser may follow
// without a person: prefetched, crawled, opened by anything walking the page.
// Delete behind a GET is a deletion that can happen because something read the
// page.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Nothing offers to delete an app with a link somebody can simply follow.
func TestNoDeleteIsReachableByFollowingALink(t *testing.T) {
	src := source(t)

	// An href pointing at the delete route. The control posts, so the only
	// href it carries is "#".
	link := regexp.MustCompile(`href="/apps/[^"]*/delete`)
	if found := link.FindAllString(src, -1); len(found) > 0 {
		t.Errorf("%d delete control(s) are ordinary links, so clicking one is a GET "+
			"and the POST-only route answers 404:\n  %s",
			len(found), strings.Join(found, "\n  "))
	}
}

// And the control that is there does post.
func TestTheDeleteControlPosts(t *testing.T) {
	got := deleteLink("live-sports", "")

	for _, want := range []string{
		"/apps/live-sports/delete", // the route
		"method:'POST'",            // as a write
		"confirm(",                 // and not without asking
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the delete control is missing %q: %s", want, got)
		}
	}
	// A slug is a path segment and reaches this from stored data, so it is
	// escaped rather than trusted.
	if q := deleteLink(`a"b`, ""); strings.Contains(q, `a"b`) {
		t.Errorf("the slug is not escaped into the attribute: %s", q)
	}
}

// It says so when it fails, rather than appearing to work.
//
// The fetch that already existed navigated to /apps on any answer at all, so a
// 403 or a 500 looked exactly like a success: the row was gone from view
// because the page had changed, and the app was still there on the next load.
func TestAFailedDeleteIsNotSilent(t *testing.T) {
	got := deleteLink("x", "")
	if !strings.Contains(got, "r.ok") {
		t.Error("the delete control navigates away whatever the server said, so a " +
			"refusal is indistinguishable from a deletion")
	}
}

func source(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	for _, f := range []string{"apps.go", "editor.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	return all.String()
}
