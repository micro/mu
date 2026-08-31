package chat

// /chat is the rooms there are, and the ones that are yours are on it.
//
// A private room is unlisted by construction — nothing links to it and knowing
// its name gets a stranger nothing. That is the security property and it was
// also the usability hole: the conversation you had with somebody yesterday was
// reachable only by going back to their profile and pressing Chat again, which
// is a way to reach a person and not a way to reach a conversation.
//
// The list is where the two meet, so both halves have to hold at once: yours are
// on it, and only yours.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
)

func list(t *testing.T, who string) string {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/chat", nil)
	if who != "" {
		sess, err := auth.CreateSession(who)
		if err != nil {
			t.Fatal(err)
		}
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	}
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /chat = %d", w.Code)
	}
	return w.Body.String()
}

// Your conversations are on your list, named for the person you are talking to.
func TestYourConversationsAreOnTheList(t *testing.T) {
	const mine, them = "lmine", "lthem"
	for _, id := range []string{mine, them} {
		auth.Create(&auth.Account{ID: id, Name: id}) //nolint:errcheck
	}
	room := PairRoom(mine, them)
	Open(room, mine, them)

	got := list(t, mine)
	if !strings.Contains(got, `href="/chat?id=`+room+`"`) {
		t.Errorf("the conversation with @%s is not on @%s's list — it is reachable "+
			"only from their profile:\n%s", them, mine, got)
	}
	// Named for who you are talking to. pairTitle names everybody in the room,
	// which on your own list means every line starts with your own name.
	if !strings.Contains(got, "@"+them) {
		t.Errorf("the conversation is not named for @%s:\n%s", them, got)
	}
	if strings.Contains(got, "@"+mine+" and ") || strings.Contains(got, " and @"+mine) {
		t.Errorf("@%s's own name is in the title of their own conversation:\n%s", mine, got)
	}

	// And the other side sees it as theirs, named the other way round.
	back := list(t, them)
	if !strings.Contains(back, `href="/chat?id=`+room+`"`) {
		t.Errorf("@%s cannot see the same conversation:\n%s", them, back)
	}
	if !strings.Contains(back, "@"+mine) {
		t.Errorf("@%s's list does not name @%s:\n%s", them, mine, back)
	}
}

// And nobody else's are.
//
// The list is built from the membership record, which is the same record the
// door checks. If it ever drifts from that, this is what fails: a room id on the
// page is a room id somebody can type.
func TestTheListIsOnlyYourConversations(t *testing.T) {
	const a, b, snoop = "lpaira", "lpairb", "lsnoop"
	for _, id := range []string{a, b, snoop} {
		auth.Create(&auth.Account{ID: id, Name: id}) //nolint:errcheck
	}
	room := PairRoom(a, b)
	Open(room, a, b)

	for _, who := range []string{snoop, ""} {
		got := list(t, who)
		if strings.Contains(got, room) {
			t.Errorf("the room between @%s and @%s is on the list for %q:\n%s",
				a, b, who, got)
		}
		for _, name := range []string{a, b} {
			if strings.Contains(got, "@"+name) {
				t.Errorf("%q is told @%s is talking to somebody:\n%s", who, name, got)
			}
		}
	}
}
