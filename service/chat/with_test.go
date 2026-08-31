package chat

// /chat?with=henrik — the conversation between you and one person.
//
// A door rather than an id. PairRoom sorts the two names so both of you reach
// the same room, and nobody should have to know that to link to it.
//
// This is the one place a private room comes into being from a request, and it
// can, because the request names the person asking as one of its two members —
// which is exactly what TestGuessingDoesNotCreateThePrivateRoom says a request
// for a room you are not in must never do.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
)

func openWith(t *testing.T, who, with string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/chat?with="+with, nil)
	if who != "" {
		sess, err := auth.CreateSession(who)
		if err != nil {
			t.Fatal(err)
		}
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	}
	w := httptest.NewRecorder()
	Handler(w, r)
	return w
}

// Both people reach the same room, and both are in it.
func TestOpeningAChatWithSomebodyMakesOneRoomForBoth(t *testing.T) {
	const mine, them = "wmine", "wthem"
	for _, id := range []string{mine, them} {
		auth.Create(&auth.Account{ID: id, Name: id}) //nolint:errcheck
	}

	w := openWith(t, mine, them)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("opening a chat answered %d, want a redirect into the room", w.Code)
	}
	to := w.Header().Get("Location")
	room := PairRoom(mine, them)
	if !strings.Contains(to, room) {
		t.Fatalf("sent to %q, want the pair room %q", to, room)
	}

	// The room now exists in the sense that matters: both are members, so both
	// get in and nobody else does.
	for _, who := range []string{mine, them} {
		if !Member(room, who) {
			t.Errorf("@%s is not in the room they were just sent to", who)
		}
	}
	if Member(room, "wsnoop") {
		t.Error("opening a chat between two people let a third in")
	}

	// And the other side reaches the same one rather than a second.
	back := openWith(t, them, mine)
	if got := back.Header().Get("Location"); !strings.Contains(got, room) {
		t.Errorf("the other side was sent to %q, want the same room %q", got, room)
	}
	if n := len(Members(room)); n != 2 {
		t.Errorf("the room has %d members after both sides opened it, want 2", n)
	}
}

// The cases that are not a conversation.
func TestOpeningAChatWithNobody(t *testing.T) {
	const who = "wsolo"
	auth.Create(&auth.Account{ID: who, Name: who}) //nolint:errcheck

	// Yourself. There is no conversation between one person, and your own
	// record is the inbox.
	if got := openWith(t, who, who).Header().Get("Location"); got != "/inbox" {
		t.Errorf("opening a chat with yourself went to %q, want /inbox", got)
	}
	// Somebody who does not exist, rather than a room named after a typo.
	if w := openWith(t, who, "nobodyhere"); w.Code != http.StatusNotFound {
		t.Errorf("a chat with an account that does not exist answered %d", w.Code)
	}
	// Signed out, before anything is created.
	w := openWith(t, "", who)
	if w.Code == http.StatusSeeOther && strings.Contains(w.Header().Get("Location"), "dm_") {
		t.Error("a signed-out caller opened a private room")
	}
}
