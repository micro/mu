package chat

// Chat is where people talk to each other.
//
// It used to be two things sharing a URL: discussion rooms, and a question box
// that asked a model and had no other participant in it. The second was the
// assistant, left over from before /agent existed, and it is the thing these
// tests pin down as gone — because it is the kind of thing that grows back.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// Without a room id, /chat is All: the rooms there are.
//
// It redirected to the lobby for a while, because a chat needs somewhere to put
// you and a directory of conversations is none of them. It has somewhere now
// and it is not this page — the lobby is the panel over Home, next to the names
// of the people in it — so this page is free to answer the question somebody
// actually brings to it, which is what else there is.
//
// The redirect is the part that must not come back: clicking Chat loaded a page
// whose only job was to send the browser somewhere else, which is the flicker.
func TestChatListsTheRooms(t *testing.T) {
	topics = []string{"Dev", "World"}

	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /chat = %d, want the list", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{`href="/chat?id=chat_Dev"`, `href="/chat?id=chat_World"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the list does not reach %s", want)
		}
	}
}

// And the lobby is one of them, under a name.
//
// It was excluded here because its only door was a panel over Home, beside a
// strip of who else was on the instance. That whole block has gone — a room
// with strangers in it on the screen somebody opens every day was the Discord
// failure, people arriving, seeing that others arrived, and saying nothing —
// so the room it opened had no way in at all.
//
// It is an ordinary room on the page of rooms now, which is what the comment
// by lobbyTopic has always said it is. Named General: "Lobby" is the id
// showing through, and "Home" was the name it had while it was a panel over
// Home.
func TestTheLobbyIsAnOrdinaryRoom(t *testing.T) {
	topics = []string{lobbyTopic, "Dev"}
	defer func() { topics = []string{"Dev", "World"} }()

	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat", nil))
	body := rr.Body.String()

	if !strings.Contains(body, `href="/chat?id=`+lobbyID+`"`) {
		t.Errorf("the room with no subject has no way in — the panel that used to "+
			"open it has gone:\n%s", body)
	}
	// The name, read out of the rooms block rather than the whole page: the
	// nav carries a Home link on every page and would match a bare search for
	// the old name.
	rooms := body
	if i := strings.Index(rooms, `<div class="rooms">`); i >= 0 {
		rooms = rooms[i:]
	}
	if !strings.Contains(rooms, "General") {
		t.Errorf("the lobby is listed and not named:\n%s", rooms)
	}
	for _, showing := range []string{">lobby<", ">Lobby<", ">Home<"} {
		if strings.Contains(rooms, showing) {
			t.Errorf("the room is listed as %s — that is the id, or the name it had "+
				"as a panel over Home:\n%s", showing, rooms)
		}
	}
}

// And from any room there is a way to the others.
//
// A chat with no way to switch channel is one room somebody found by a link.
// The row is the same .head strip news and video use for their topics, because
// switching channel and switching topic are the same gesture.
func TestTheChannelRowReachesTheOtherRooms(t *testing.T) {
	topics = []string{"Dev", "World"}

	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat?id="+lobbyID, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET the lobby = %d", rr.Code)
	}
	body := rr.Body.String()

	for _, want := range []string{`href="/chat?id=chat_Dev"`, `href="/chat?id=chat_World"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the channel row does not reach %s", want)
		}
	}
	// All is the way back to the list, and is /chat itself rather than a room.
	if !strings.Contains(body, `<a class="head" href="/chat">All</a>`) {
		t.Errorf("the channel row has no way back to the list:\n%s", body)
	}

	// The room you are in is marked and is not a link to itself.
	rr = httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat?id=chat_Dev", nil))
	body = rr.Body.String()
	if !strings.Contains(body, `<span class="head head-on">Dev</span>`) {
		t.Errorf("the current channel is not marked:\n%s", body)
	}
	if strings.Contains(body, `href="/chat?id=chat_Dev"`) {
		t.Error("the room links to itself")
	}
}

// The lobby is the one room with no subject, so nothing draws an About block
// over the messages.
func TestTheLobbyIsAboutNothing(t *testing.T) {
	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat?id="+lobbyID, nil))

	if body := rr.Body.String(); strings.Contains(body, "room-about") {
		t.Errorf("the lobby drew an About block:\n%s", body)
	}
}

// A program with no room id still gets the catalogue, because "what rooms are
// there" is a real question for something that cannot click.
func TestAJSONCallerStillGetsTheList(t *testing.T) {
	topics = []string{"Dev", "World"}

	r := httptest.NewRequest("GET", "/chat", nil)
	r.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	Handler(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /chat as JSON = %d, want 200", rr.Code)
	}
	for _, want := range []string{"Dev", "World"} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Errorf("the catalogue is missing %q", want)
		}
	}
}

// The question box is what made chat and the agent the same thing. A page that
// still shipped one would be shipping two assistants again.
//
// chat-form is not in this list any more and must not go back: it is the room's
// own box, which sends a websocket frame to the people present. It was banned
// while this page was a catalogue and a catalogue has no box.
func TestNoRoomCarriesTheOldQuestionBox(t *testing.T) {
	topics = []string{"Dev"}

	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat?id="+lobbyID, nil))

	body := rr.Body.String()
	for _, gone := range []string{"topic-selector", "askLLM"} {
		if strings.Contains(body, gone) {
			t.Errorf("a room still carries %q, which belonged to the question box", gone)
		}
	}
}

// Saying something into a room is a websocket frame. The POST that used to be
// here spent a model call per request, so leaving it reachable would mean an
// unauthenticated-looking path that costs money.
func TestPostingToChatIsRefused(t *testing.T) {
	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("POST", "/chat", strings.NewReader("prompt=hello")))

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /chat = %d, want 405", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "/agent") {
		t.Error("the refusal should say where to ask a question instead")
	}
}

// A room page is the conversation and a box to say something into it, and the
// box must not post anywhere — mu.js binds it to the websocket.
func TestARoomPageSendsOverTheWebsocketNotAForm(t *testing.T) {
	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat?id=news_42", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /chat?id=news_42 = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `id="chat-form"`) {
		t.Fatal("a room needs a box to say something into")
	}
	if strings.Contains(body, "askLLM(this)") {
		t.Error("the room form still submits to the model")
	}
	if !strings.Contains(body, `id="room-data"`) {
		t.Error("the room page must hand mu.js the room to connect to")
	}

	// Inside the content, which is the part soft navigation carries over.
	//
	// It used to be a script appended before </body>, outside #content, so
	// arriving here by clicking "Join discussion" swapped in a room page whose
	// room was never handed over — and the only fix a person could find was a
	// hard refresh. Checked by position rather than presence, because presence
	// was already true while the bug was live.
	form := strings.Index(body, `id="chat-form"`)
	room := strings.Index(body, `id="room-data"`)
	if form < 0 || room < form {
		// Both are in the content, and the content is one contiguous block, so
		// the room data following the form is enough to place it inside.
		if end := strings.Index(body, "</body>"); end >= 0 && room > end {
			t.Error("the room data is outside the body, where a soft navigation cannot reach it")
		}
	}
	if strings.Contains(body, "var roomData") {
		t.Error("the room is still a global — it outlives the page that set it, " +
			"so navigating away leaves the next page holding the last room")
	}
}

// The card is what the home screen shows, and on a quiet instance it shows
// nothing at all.
//
// It used to say "No discussions going on right now" — which is a card whose
// content is the absence of content, taking the same space as a real one. A
// card is evidence that a service is doing something; one that reports nothing
// to report is evidence of the opposite. On a fresh account four of the ten
// cards read like this, and Home looked broken rather than new.
//
// Home skips a card whose renderer returns empty. That is the contract, and
// this is chat honouring it.
func TestAQuietInstanceRendersNoChatCard(t *testing.T) {
	roomsMutex.Lock()
	saved := rooms
	rooms = map[string]*Room{}
	roomsMutex.Unlock()
	defer func() {
		roomsMutex.Lock()
		rooms = saved
		roomsMutex.Unlock()
	}()

	if got := Card(); got != "" {
		t.Errorf("card on a quiet instance = %q, want nothing rendered", got)
	}
}

// Who is in the room is people.
//
// This asserted the opposite — that the agent is on every roster, because it
// answers in every room. The argument was about a real disagreement (the list
// included it for chat_ rooms only, while the rule about when it replies says
// an item room always gets one) and it fixed the wrong side.
//
// Reported by somebody who started a conversation with one other person and
// found @micro in it: announcing their arrival, listed as present, and
// answering. A two-person conversation with a third party on the roster is the
// product telling you somebody is in the room with you.
//
// It is the same rule the strip on Home already applies — filter on
// acc.Agent — and holding it in one place and not the other is how @micro came
// to be listed here and not there. Being reachable by name is not presence.
func TestTheAgentIsNotOnTheRoster(t *testing.T) {
	for _, id := range []string{
		"news_123", "video_123", "post_123", "reminder_daily", "chat_Dev",
		PairRoom("rosta", "rostb"),
	} {
		room := &Room{ID: id, Clients: map[*websocket.Conn]*Client{}}
		for _, name := range room.roster() {
			if name == agentName {
				t.Errorf("%s lists %v — @%s is not a person and this says it is here",
					id, room.roster(), agentName)
			}
		}
	}

	// And the people who are connected are all still on it. Dropping the agent
	// must not drop anybody else: the loop that skips it is the loop that
	// builds the list.
	room := &Room{ID: "chat_Dev", Clients: map[*websocket.Conn]*Client{}}
	for _, id := range []string{"ros1", "ros2"} {
		room.Clients[&websocket.Conn{}] = &Client{UserID: id}
	}
	got := room.roster()
	if len(got) != 2 {
		t.Fatalf("roster() = %v, want the two people connected", got)
	}
}
