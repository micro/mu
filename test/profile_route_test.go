package test

// /@username is the profile, not the user service.
//
// internal/user held the public profile page and service/user holds what an
// account saves, hides and blocks. Two packages called user, and the file that
// dispatches /@username imported one of them under that name. Renaming
// internal/user to internal/profile did not break the build, because the other
// user was already imported in the same file — so /@somebody quietly started
// rendering somebody else's saved-items page.
//
// Nothing caught it. The compiler could not: both packages export Handler with
// the same signature.

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

	if !strings.Contains(branch, "profile.Handler(w, r)") {
		t.Error("/@username is not served by internal/profile")
	}
	if strings.Contains(branch, "user.Handler(w, r)") {
		t.Error("/@username is served by the user service, which renders what " +
			"the caller has saved, hidden and blocked — not a public profile")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
