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

// Without a room id, a person goes to the lobby.
//
// This used to answer with the catalogue: every topic, a paragraph of summary
// under each, and a Join link. Nobody arrives at a chat wanting to read about
// rooms — a directory of conversations is none of them — so the list stood
// between somebody and the only thing on it worth doing.
func TestChatDropsYouIntoTheLobby(t *testing.T) {
	topics = []string{"Dev", "World"}

	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat", nil))

	if rr.Code != http.StatusFound {
		t.Fatalf("GET /chat = %d, want a redirect", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/chat?id="+lobbyID {
		t.Errorf("GET /chat went to %q, want the lobby", got)
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
	// The room you are in is marked and is not a link to itself.
	if !strings.Contains(body, `<span class="head head-on">Lobby</span>`) {
		t.Errorf("the current channel is not marked:\n%s", body)
	}
	if strings.Contains(body, `href="/chat?id=`+lobbyID+`"`) {
		t.Error("the lobby links to itself")
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

// The agent is in the room it answers in.
//
// "Discuss with AI" on a news article listed one person present — you — and
// then the AI answered. The list included it for chat_ rooms only, while the
// rule about when it replies is written elsewhere and says an item room always
// gets one. Two conditions about the same fact, apart, disagreeing.
func TestTheAgentIsInEveryRoom(t *testing.T) {
	for _, id := range []string{"news_123", "video_123", "post_123", "reminder_daily", "chat_Dev"} {
		room := &Room{ID: id, Clients: map[*websocket.Conn]*Client{}}
		found := false
		for _, name := range room.roster() {
			if name == agentName {
				found = true
			}
		}
		if !found {
			t.Errorf("%s lists %v — the agent answers here and is not in the room",
				id, room.roster())
		}
	}
}
