package test

// /@username is the conversation with them, and there is no profile.
//
// It served a profile page — a name, a tick, a join date, a status box, an apps
// grid and their posts. That is a social network's page, and this is not one:
// internal/app/content.go deleted Save, Hide and Block on the grounds that
// "those three are the controls of a feed… Mu has no feed", and a profile is
// downstream of the same thing.
//
// What the address is for survives the page. "Everything has an address —
// people, agents, services, conversations" is one of four commitments, and an
// address is worth what it lets you do. So /@somebody is what the two of you
// have said to each other, with the way to say the next thing.
//
// The wiring is held here because the hazard that made this file necessary is
// still live: the dispatch for /@ sits in a chain of prefix checks in serve.go,
// several handlers with identical signatures are in scope, and swapping one for
// another breaks nothing a compiler can see.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheAtRouteServesTheConversation(t *testing.T) {
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
func TestThereIsNoProfilePageLeft(t *testing.T) {
	for _, gone := range []string{
		filepath.Join("internal", "user", "profile.go") + ":ProfileHandler",
		filepath.Join("internal", "user", "status.go") + ":",
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

	// The nav offered it, and a link to a page that no longer exists is worse
	// than no link.
	shell, err := os.ReadFile(at("internal", "app", "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(shell), `id="nav-profile"`) {
		t.Error("the sidebar still offers Profile")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
