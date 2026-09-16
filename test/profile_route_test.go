package test

// /@username uses the account-scoped person view. Profile settings live under
// /account/profile; the retired social-profile handler must not return.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheAtRouteServesThePerson(t *testing.T) {
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

	if !strings.Contains(branch, "inbox.PersonHandler(w, r)") {
		t.Error("/@username is not served by the conversation handler")
	}
	if strings.Contains(branch, "user.ProfileHandler") {
		t.Error("/@username still serves a profile page")
	}
	if strings.Contains(branch, "user.Handler(w, r)") {
		t.Error("/@username is served by the page that renders what the caller has " +
			"saved, hidden and blocked — that is theirs alone")
	}
}

// And the profile is gone rather than merely unreachable.
//
// An unrouted page is a page somebody re-routes in six months without knowing
// why it was taken out. The handler, the status it carried and the two hooks
// that fed it their posts and apps all went with it.
func TestLegacyProfileIsRetiredAndSettingsAreReachable(t *testing.T) {
	for _, gone := range []string{
		filepath.Join("internal", "user", "profile.go") + ":ProfileHandler",
		filepath.Join("internal", "user", "post.go") + ":",
	} {
		parts := strings.SplitN(gone, ":", 2)
		path, symbol := at(strings.Split(parts[0], string(filepath.Separator))...), parts[1]

		src, err := os.ReadFile(path)
		if err != nil {
			continue // the whole file went, which is the strongest version of gone
		}
		if symbol == "" {
			t.Errorf("%s still exists; the profile it served does not", parts[0])
			continue
		}
		if strings.Contains(string(src), "func "+symbol) {
			t.Errorf("%s still defines %s", parts[0], symbol)
		}
	}

	// The menu opens the authenticated profile settings, not the retired page.
	shell, err := os.ReadFile(at("internal", "app", "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shell), `id="nav-account" href="/account"`) {
		t.Error("Settings must open the consolidated account page")
	}

}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
