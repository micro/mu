package user

// What a profile page is: who this is and how to reach them.

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

// renderProfile serves /@name for an account and returns the HTML.
func renderProfile(t *testing.T, id string) string {
	t.Helper()
	if _, err := auth.GetAccount(id); err != nil {
		t.Skipf("no account %q on this machine to render", id)
	}
	rec := httptest.NewRecorder()
	ProfileHandler(rec, httptest.NewRequest("GET", "/@"+id, nil))
	return rec.Body.String()
}

// The address is on the page.
//
// It was computed here all along — writeLink asks addressOf whether mail can
// leave at all — and thrown away, so the one fact a directory entry exists to
// carry was the one thing the page did not print. The stylesheet even said it
// was shown.
func TestTheProfileShowsTheAddress(t *testing.T) {
	prev := AddressFor
	AddressFor = func(id string) string { return id + "@example.test" }
	t.Cleanup(func() { AddressFor = prev })

	for _, acc := range auth.AllAccounts() {
		if acc.Banned {
			continue
		}
		body := renderProfile(t, acc.ID)
		if !strings.Contains(body, acc.ID+"@example.test") {
			t.Errorf("/@%s does not show the address it already knows", acc.ID)
		}
		return
	}
	t.Skip("no accounts on this machine to render")
}

// No tallies beside somebody's name.
//
// "Posts (47)" and "Apps (3)" are the social-media profile in miniature: a
// number next to a person, which nobody needs and which turns the page into a
// scoreboard. The headings stay, the counts go.
//
// The apps and posts are stubbed in rather than hoped for. A first version of
// this rendered whatever account came first, which had neither, so the whole
// assertion ran against a page with no sections on it and passed with the
// counts put back.
func TestTheProfileCountsNothing(t *testing.T) {
	prevAddr, prevApps, prevPosts := AddressFor, GetUserApps, GetUserPosts
	t.Cleanup(func() { AddressFor, GetUserApps, GetUserPosts = prevAddr, prevApps, prevPosts })

	AddressFor = func(id string) string { return id + "@example.test" }
	GetUserApps = func(string) []UserApp {
		return []UserApp{
			{Slug: "one", Name: "App One", Description: "first"},
			{Slug: "two", Name: "App Two", Description: "second"},
			{Slug: "three", Name: "App Three", Description: "third"},
		}
	}
	GetUserPosts = func(string, string) []UserPost {
		return []UserPost{
			{ID: "p1", Title: "First", Content: "body", CreatedAt: time.Now()},
			{ID: "p2", Title: "Second", Content: "body", CreatedAt: time.Now()},
		}
	}

	body := renderAnyProfile(t)

	// Present, so the assertion below is about how they are rendered rather
	// than about a page that rendered nothing.
	for _, want := range []string{"App One", "First", "Apps", "Posts"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the page does not contain %q, so this proves nothing:\n%s", want, body)
		}
	}
	for _, tally := range []string{"Apps (", "Posts (", "Apps (3)", "Posts (2)"} {
		if strings.Contains(body, tally) {
			t.Errorf("renders %q — a count beside a name is a scoreboard", tally)
		}
	}
	// The body of a post belongs at the other end of the link, not here: a
	// truncated body under every title is a feed, and a feed on a person is
	// the shape this page is getting out of.
	if strings.Contains(body, "Read more") {
		t.Error("posts still render as feed items with a Read more")
	}
}

// renderAnyProfile renders the first account this machine has.
func renderAnyProfile(t *testing.T) string {
	t.Helper()
	for _, acc := range auth.AllAccounts() {
		if acc.Banned {
			continue
		}
		return renderProfile(t, acc.ID)
	}
	t.Skip("no accounts on this machine to render")
	return ""
}
