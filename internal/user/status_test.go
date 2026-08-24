package user

// A profile people open twice, and where Write goes.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func person(t *testing.T, id string) {
	t.Helper()
	if _, err := auth.GetAccount(id); err == nil {
		return
	}
	if err := auth.Create(&auth.Account{ID: id, Name: id, Created: time.Now()}); err != nil {
		t.Fatalf("could not create %s: %v", id, err)
	}
}

// A status survives being set, which it could not before.
//
// The store was loaded at init and written by nothing at all, so the field it
// held had no way of outliving a restart — which is most of why it stopped
// being set and then stopped existing.
func TestAStatusIsKept(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "status-keeper"

	SetStatus(who, "Building the sandbox")
	got, at := Status(who)
	if got != "Building the sandbox" {
		t.Errorf("read back %q", got)
	}
	if at.IsZero() {
		t.Error("nothing recorded when it was said, so the profile cannot say how stale it is")
	}

	// Emptying it clears it rather than leaving the old one standing.
	SetStatus(who, "   ")
	if got, _ := Status(who); got != "" {
		t.Errorf("clearing left %q", got)
	}

	// And it is bounded — a status is a line about what you are doing, not a post.
	SetStatus(who, strings.Repeat("x", statusLimit*3))
	if got, _ := Status(who); len([]rune(got)) > statusLimit {
		t.Errorf("a status of %d runes was kept whole", len([]rune(got)))
	}
}

// Write goes to the person, and does not say what their address is.
//
// service/mail draws the first line already: untagged mail to an address is
// just mail and wakes nothing, while a tag is an agent's. So this must not
// carry a "+" — that would quietly run somebody's agent instead of writing to
// them.
//
// The second line is newer and is the one that had been crossed. This printed
// the address beside the button and carried it in the href, and /@somebody is
// a public page — so every account's mailbox was published to anybody who
// opened it and anything that crawled it. Nobody chose to advertise it; it is
// how the product works. The button names the person now and inbox/new
// resolves it on the way out, so the shortcut survives and the address does
// not leave the server.
func TestWriteNamesThePersonAndNotTheirAddress(t *testing.T) {
	AddressFor = func(id string) string { return id + "@example.test" }
	t.Cleanup(func() { AddressFor = nil })

	got := writeLink("henrik")
	if !strings.Contains(got, "/inbox/new?to=%40henrik") {
		t.Errorf("Write does not open a message to them:\n%s", got)
	}
	if strings.Contains(got, "%2B") || strings.Contains(got, "+") {
		t.Errorf("the address carries a tag, so this writes to an agent:\n%s", got)
	}
	// The whole point: no address anywhere on it, in the text or in the link.
	if strings.Contains(got, "henrik@example.test") || strings.Contains(got, "%40example.test") {
		t.Errorf("the address is published on a public page:\n%s", got)
	}

	// An instance with no mail offers no button rather than a broken one.
	AddressFor = nil
	if got := writeLink("henrik"); got != "" {
		t.Errorf("a Write button was drawn with nowhere to send: %s", got)
	}
}

// Your own profile is where you change what it says. Somebody else's is not.
func TestOnlyYouCanSetYourOwnStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "status-owner"
	person(t, who)
	SetStatus(who, "Reading")

	if mine := statusBlock(who, true, "tok"); !strings.Contains(mine, `action="/profile/status"`) ||
		!strings.Contains(mine, "Reading") {
		t.Errorf("your own status is not editable where it is read:\n%s", mine)
	}
	if theirs := statusBlock(who, false, ""); strings.Contains(theirs, "<form") {
		t.Errorf("somebody else's status came with a box to change it:\n%s", theirs)
	}

	// And the POST needs a session, or a status is something anybody can set.
	rec := httptest.NewRecorder()
	StatusHandler(rec, httptest.NewRequest("POST", "/status", nil))
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/login") {
		t.Errorf("a stranger got %d rather than the sign-in page", rec.Code)
	}
}
