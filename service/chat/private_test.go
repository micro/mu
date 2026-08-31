package chat

// A room nobody can walk into by guessing its name.
//
// The attack is not joining somebody's room. It is creating it:
// getOrCreateRoom makes a room for any id it is handed, so asking for an id
// nobody has used yet hands back a fresh room with that name — and when the two
// people it belongs to open theirs, they join the guesser's.
//
// That is why privacy is a property of the name rather than a flag on a room.
// The check has to be answerable before any room exists.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
)

// The pair id is the same from either side, and is private by its name alone.
func TestAPairRoomIsTheSameRoomFromBothSides(t *testing.T) {
	one := PairRoom("asim", "henrik")
	other := PairRoom("henrik", "asim")
	if one == "" || one != other {
		t.Fatalf("asim→henrik is %q and henrik→asim is %q; two people have one "+
			"conversation between them", one, other)
	}
	if !Private(one) {
		t.Errorf("%q is a conversation between two people and does not read as "+
			"private, so getOrCreateRoom would make it for anybody who asked", one)
	}
	// Nonsense in, nothing out — rather than a room called dm__ that anybody
	// could ask for.
	for _, pair := range [][2]string{{"", "henrik"}, {"asim", ""}, {"asim", "asim"}} {
		if got := PairRoom(pair[0], pair[1]); got != "" {
			t.Errorf("PairRoom(%q, %q) invented %q", pair[0], pair[1], got)
		}
	}
}

// A public room is open to everybody, which is what public means.
func TestAPublicRoomAdmitsAnybody(t *testing.T) {
	for _, id := range []string{"chat_lobby", "chat_finance", "post_123", "news_456"} {
		if Private(id) {
			t.Errorf("%q reads as private; a topic room comes into being because "+
				"somebody opened it and that is the feature", id)
		}
		if !Member(id, "anybody") {
			t.Errorf("%q turned somebody away from a public room", id)
		}
	}
}

// Membership is what a private room has instead of a lock on the door.
func TestOnlyTheMembersOfAPrivateRoom(t *testing.T) {
	room := PairRoom("privmine", "privthem")

	// Before anybody opens it, nobody is in it — including the two people whose
	// names are in the id. Knowing the name is not membership, which is the
	// whole point of the id being guessable.
	if Member(room, "privmine") {
		t.Error("a room nobody has opened already has a member in it")
	}

	Open(room, "privmine", "privthem")
	for _, who := range []string{"privmine", "privthem"} {
		if !Member(room, who) {
			t.Errorf("@%s opened this room and is not in it", who)
		}
	}
	if Member(room, "privsnoop") {
		t.Error("somebody who was never invited is a member of a private room")
	}
	if Member(room, "") {
		t.Error("a signed-out caller is a member of a private room")
	}

	// Opening twice is the second person arriving, not a second room, and not a
	// duplicate member.
	Open(room, "privmine")
	if n := len(Members(room)); n != 2 {
		t.Errorf("the room has %d members after being opened twice, want 2: %v",
			n, Members(room))
	}

	// And an invite is the same operation.
	Open(room, "privguest")
	if !Member(room, "privguest") {
		t.Error("an invited account is not a member")
	}
}

// The handler refuses before it creates, and refuses by saying there is nothing
// there.
//
// Both halves matter. Ordering is the fix — a check that ran after
// getOrCreateRoom would be checking a room the guesser had just made. And "you
// are not a member of dm_asim_henrik" confirms that asim and henrik are
// talking, which is most of what there was to learn.
func TestGuessingAPrivateRoomFindsNothing(t *testing.T) {
	const mine, them, snoop = "hmine", "hthem", "hsnoop"
	for _, id := range []string{mine, them, snoop} {
		auth.Create(&auth.Account{ID: id, Name: id}) //nolint:errcheck
	}
	room := PairRoom(mine, them)
	Open(room, mine, them)

	ask := func(who string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/chat?id="+room, nil)
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

	if w := ask(snoop); w.Code != http.StatusNotFound {
		t.Errorf("a stranger asking for a private room got %d, want 404", w.Code)
	}
	if w := ask(""); w.Code != http.StatusNotFound {
		t.Errorf("a signed-out caller asking for a private room got %d, want 404",
			w.Code)
	}
	// Refused by saying it is not there, never by naming who is in it.
	//
	// The names and the room id, and the phrasings that would confirm the room
	// exists. Not a bare search for "member": the page shell contains the word
	// inside "remember", and a check that fires on that is a check that will be
	// loosened by the next person rather than tightened.
	body := ask(snoop).Body.String()
	for _, leak := range []string{mine, them, room, "not a member", "Not a member",
		"not allowed", "forbidden", "Forbidden"} {
		if strings.Contains(body, leak) {
			t.Errorf("the refusal leaks %q, which tells a stranger the room "+
				"exists or who is in it", leak)
		}
	}

	// A member is not refused. It gets as far as the room, which is all this
	// test claims — what the room then renders is the rest of the handler.
	if w := ask(mine); w.Code == http.StatusNotFound {
		t.Error("a member of the room was told there is no such room")
	}
}

// The guesser cannot create it either, which is the half a flag on Room misses.
func TestGuessingDoesNotCreateThePrivateRoom(t *testing.T) {
	const a, b, snoop = "cmine", "cthem", "csnoop"
	for _, id := range []string{a, b, snoop} {
		auth.Create(&auth.Account{ID: id, Name: id}) //nolint:errcheck
	}
	room := PairRoom(a, b)

	sess, err := auth.CreateSession(snoop)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/chat?id="+room, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	Handler(httptest.NewRecorder(), r)

	roomsMutex.RLock()
	_, exists := rooms[room]
	roomsMutex.RUnlock()
	if exists {
		t.Error("asking for a private room brought it into existence — the two " +
			"people it belongs to would now be joining the guesser's room")
	}
	if Member(room, snoop) {
		t.Error("asking for a room made the asker a member of it")
	}
}
