package apps

import (
	"strings"
	"testing"
)

// An app always has an address, including one loaded from an old record.
//
// Records written before the slug field existed load with it empty, and an app
// with no slug is worse than a missing app: it appears in every list, with a
// link to /apps/ or /code?app= that looks live and goes nowhere. Worse still,
// apps are keyed by slug on load, so the second slugless record silently
// replaces the first.
//
// The repair is at load, not at the pages, because every page that lists an app
// builds a link from its slug and fixing them one at a time means finding all
// of them — and the next page somebody writes starts the bug again.
func TestAnAppLoadedWithNoSlugGetsOne(t *testing.T) {
	for _, tc := range []struct{ name, id, want string }{
		{"Prayer Tracker", "abc", "prayer-tracker"},
		// A name with nothing sluggable in it still has to land somewhere,
		// and the id is the one thing guaranteed unique.
		{"🙏", "xyz", "app-xyz"},
		// And a name that slugs to something too short for the slug rule.
		{"Go", "def", "app-go"},
	} {
		got := repairSlug(&App{Name: tc.name, ID: tc.id})
		if got != tc.want {
			t.Errorf("an app called %q got the address %q, want %q", tc.name, got, tc.want)
		}
		if len(got) < 3 {
			t.Errorf("%q is shorter than apps will accept", got)
		}
		if strings.ContainsAny(got, " /?&#") {
			t.Errorf("%q is not usable in a URL", got)
		}
	}
}
