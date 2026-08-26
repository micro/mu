package test

// Every admin page is reachable by clicking.
//
// This exists because a Federation check was added to /admin/diagnostics, which
// works, is registered, is covered by tests, and was linked from nowhere. Ten
// links on /admin and that was not one of them, so the only way to the page was
// to already know the URL — which is the same as not having built it, for
// anybody who is not the person who wrote it.
//
// Route registration and the menu are two lists in two files, and nothing made
// them agree. This is the thing that makes them agree.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// notAPage is an /admin route that correctly has no menu entry, and why.
//
// Each of these is reached by doing something rather than by going somewhere. A
// menu entry for one would be a link to a page that does not exist, or a link
// that performs an action on arrival, and both are worse than the absence.
var notAPage = map[string]string{
	"/admin":           "the menu itself",
	"/admin/flag":      "an action posted from a moderation row",
	"/admin/delete":    "an action posted from a user row",
	"/admin/invite":    "reached from /admin/users, where invites are",
	"/admin/blocklist": "a stub that redirects somewhere it moved to",
	"/admin/email":     "a stub that redirects somewhere it moved to",
	"/admin/usage":     "a stub that redirects to /admin/traffic",
}

func TestEveryAdminPageIsInTheAdminMenu(t *testing.T) {
	routes, err := os.ReadFile("../internal/server/routes.go")
	if err != nil {
		t.Fatalf("reading the routes: %v", err)
	}
	menu, err := os.ReadFile("../admin/admin.go")
	if err != nil {
		t.Fatalf("reading the admin page: %v", err)
	}

	registered := regexp.MustCompile(`http\.HandleFunc\("(/admin[^"]*)"`)
	found := registered.FindAllSubmatch(routes, -1)
	if len(found) < 10 {
		t.Fatalf("found %d admin routes, which is too few to be the real list — "+
			"the pattern has stopped matching how they are registered", len(found))
	}

	for _, m := range found {
		path := string(m[1])
		if why, ok := notAPage[path]; ok {
			// Kept honest in the other direction too: an exemption for a route
			// that no longer exists is a comment nobody will ever re-read.
			_ = why
			continue
		}
		if !strings.Contains(string(menu), `"`+path+`"`) {
			t.Errorf("%s is registered but nothing on /admin links to it, so the "+
				"only way there is to know the URL. Add it to the list in "+
				"admin/admin.go, or to notAPage here with the reason it is not one.", path)
		}
	}
}

// The exemptions describe routes that exist. One that does not is a claim
// nobody checks, left behind by a route that was renamed or deleted.
func TestTheAdminMenuExemptionsAreAllRealRoutes(t *testing.T) {
	routes, err := os.ReadFile("../internal/server/routes.go")
	if err != nil {
		t.Fatalf("reading the routes: %v", err)
	}
	for path, why := range notAPage {
		if !strings.Contains(string(routes), `"`+path+`"`) {
			t.Errorf("notAPage exempts %s (%q) but no such route is registered", path, why)
		}
	}
}
