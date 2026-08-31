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
	"time"

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

// The window on Home shows what a person said, not the agent's reply.
//
// It is drawn directly under the strip naming who is online, which the agent is
// deliberately not on. Left as "the newest message" it quoted @micro two lines
// below a list that excludes @micro — somebody present that the list forgot. The
// agent answers in every room, so that was the common case rather than an edge.
func TestTheLatestLineIsFromAPerson(t *testing.T) {
	const room = "chat_latest"
	now := time.Now()
	liveRoom(t, room, RoomMessage{UserID: "somebody", Content: "is the shop open", Timestamp: now})

	// The agent answers, which is what happens in every room.
	roomsMutex.Lock()
	rooms[room].Messages = append(rooms[room].Messages, RoomMessage{
		UserID: agentName, Content: "I need a bit more to go on", IsLLM: true,
		Timestamp: now.Add(time.Second),
	})
	roomsMutex.Unlock()

	who, text, at := Latest(room)
	if who != "somebody" {
		t.Errorf("Latest names %q — the strip above this line does not list the agent, "+
			"so quoting it reads as somebody the list forgot", who)
	}
	if text != "is the shop open" {
		t.Errorf("Latest said %q, want the person's line", text)
	}
	if at.IsZero() {
		t.Error("Latest gave no time, so the line cannot say it is the older one")
	}

	// A room where only the agent has spoken has nothing to show.
	const quiet = "chat_agentonly"
	liveRoom(t, quiet, RoomMessage{UserID: agentName, Content: "hello", IsLLM: true, Timestamp: now})
	if who, _, _ := Latest(quiet); who != "" {
		t.Errorf("Latest returned %q in a room only the agent has spoken in", who)
	}
}
