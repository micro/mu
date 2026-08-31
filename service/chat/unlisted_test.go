package chat

// A private room is not named to anybody who is not in it.
//
// The door has been checked since private rooms existed: ask for one you are not
// a member of and you are told there is no such room. That is the second half of
// the property and it was the only half implemented. Every place that assembles
// a list of what is happening — the channel row across the top of a room, the
// Happening now block, the chat_rooms tool, the card on Home — took every room
// that was not a topic and named it, and a conversation between two people is
// exactly that. So the moment they said anything, "@asim and @henrik" appeared
// on everybody else's screen, and not-found at the door is no use when the page
// in front of it announces the room.
//
// Reading and speaking were open too: chat_messages returned any room's
// transcript to anybody who could name it, and the name is two usernames by
// construction.
//
// Every door in one file, because the bug was that they were written separately.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"mu/internal/auth"
	"mu/internal/service"
)

// liveRoom puts a private room into the rooms map with something said in it, the
// way two people talking would, and takes it out again afterwards.
func liveRoom(t *testing.T, id string, said RoomMessage) {
	t.Helper()
	room := &Room{
		ID:       id,
		Type:     "dm",
		Title:    pairTitle(id),
		Clients:  map[*websocket.Conn]*Client{},
		Messages: []RoomMessage{said},
	}
	room.LastActivity = said.Timestamp
	roomsMutex.Lock()
	rooms[id] = room
	roomsMutex.Unlock()
	t.Cleanup(func() {
		roomsMutex.Lock()
		delete(rooms, id)
		roomsMutex.Unlock()
	})
}

// Not on the channel row, not in Happening now, not in the catalogue.
func TestAPrivateRoomIsNotOnAnyList(t *testing.T) {
	const a, b, snoop = "ulmine", "ulthem", "ulsnoop"
	for _, id := range []string{a, b, snoop} {
		auth.Create(&auth.Account{ID: id, Name: id}) //nolint:errcheck
	}
	room := PairRoom(a, b)
	Open(room, a, b)
	liveRoom(t, room, RoomMessage{UserID: a, Content: "the thing at nine", Timestamp: time.Now()})

	sess, err := auth.CreateSession(snoop)
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string, json bool) string {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if json {
			r.Header.Set("Accept", "application/json")
		}
		w := httptest.NewRecorder()
		Handler(w, r)
		return w.Body.String()
	}

	// The list, the JSON catalogue, and the channel row on some other room —
	// which is on every room page, so it is the widest of the three.
	for _, where := range []struct {
		what string
		body string
	}{
		{"the list at /chat", get("/chat", false)},
		{"the JSON catalogue", get("/chat", true)},
		{"the channel row on another room", get("/chat?id=chat_Dev", false)},
	} {
		if strings.Contains(where.body, room) {
			t.Errorf("%s names the room %s:\n%s", where.what, room, where.body)
		}
		if strings.Contains(where.body, "@"+a) || strings.Contains(where.body, "@"+b) {
			t.Errorf("%s tells @%s that @%s and @%s are talking:\n%s",
				where.what, snoop, a, b, where.body)
		}
	}

	// And the tool, which is also what the card on Home is built from.
	var rsp RoomsResponse
	if err := (Server{}).Rooms(context.Background(), &RoomsRequest{}, &rsp); err != nil {
		t.Fatal(err)
	}
	for _, r := range rsp.Rooms {
		if r.ID == room {
			t.Errorf("chat_rooms lists %s — the card on Home is built from this, "+
				"so it would put %q on the front page", room, r.Title)
		}
	}
}

// And its transcript is not readable, and nobody can speak into it.
func TestAPrivateRoomIsNotReadableOrWritableByAStranger(t *testing.T) {
	const a, b, snoop = "ulreada", "ulreadb", "ulreads"
	for _, id := range []string{a, b, snoop} {
		auth.Create(&auth.Account{ID: id, Name: id}) //nolint:errcheck
	}
	room := PairRoom(a, b)
	Open(room, a, b)
	const secret = "the thing at nine"
	liveRoom(t, room, RoomMessage{UserID: a, Content: secret, Timestamp: time.Now()})

	as := func(who string) context.Context {
		return service.WithAccount(context.Background(), who)
	}

	// A member reads it.
	var mine MessagesResponse
	if err := (Server{}).Messages(as(a), &MessagesRequest{Room: room}, &mine); err != nil {
		t.Fatalf("@%s cannot read their own conversation: %v", a, err)
	}
	if len(mine.Messages) == 0 {
		t.Fatal("a member read their own room and got nothing")
	}

	// Nobody else does.
	for _, who := range []string{snoop, ""} {
		var got MessagesResponse
		err := (Server{}).Messages(as(who), &MessagesRequest{Room: room}, &got)
		if err == nil {
			t.Errorf("%q read the room between @%s and @%s", who, a, b)
		}
		for _, m := range got.Messages {
			if strings.Contains(m.Content, secret) {
				t.Errorf("%q was handed the transcript", who)
			}
		}
		// And is not told the room is real, which is most of what there was to
		// learn — the id is two usernames by construction.
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "member") {
			t.Errorf("the refusal for %q confirms the room exists: %v", who, err)
		}
	}

	// And cannot say anything into it.
	var sent SendResponse
	if err := (Server{}).Send(as(snoop), &SendRequest{Room: room, Message: "hello"}, &sent); err == nil {
		t.Errorf("@%s spoke into the room between @%s and @%s", snoop, a, b)
	}
}
