package test

// /@username is the profile, not the page about what you keep.
//
// There were two packages called user — one holding the public profile page,
// one holding what an account saves, hides and blocks — and the file that
// dispatches /@username imported one of them under that name. Renaming did not
// break the build, because the other user was already imported in the same
// file, so /@somebody quietly started rendering somebody else's saved-items
// page. Nothing caught it: both packages exported Handler with the same
// signature.
//
// They are one package now, which removes the import ambiguity and leaves the
// hazard: two handlers, one public and one private, a few lines apart. The
// public one is named for its page for that reason, and this holds the wiring.

import (
	"os"
	"strings"
	"testing"
)

func TestTheProfileRouteServesTheProfile(t *testing.T) {
	src, err := os.ReadFile(at("internal", "server", "serve.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	i := strings.Index(body, `strings.HasPrefix(r.URL.Path, "/@")`)
	if i < 0 {
		t.Fatal("nothing dispatches /@username any more")
	}
	branch := body[i:min(i+2500, len(body))]

	if !strings.Contains(branch, "user.ProfileHandler(w, r)") {
		t.Error("/@username is not served by the profile handler")
	}
	if strings.Contains(branch, "user.Handler(w, r)") {
		t.Error("/@username is served by the page that renders what the caller has " +
			"saved, hidden and blocked — that is theirs alone, not a public profile")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
