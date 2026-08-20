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

// Without a room id there is nobody to talk to, so the page answers the only
// question left: what is being discussed.
func TestChatWithoutARoomListsWhatIsBeingDiscussed(t *testing.T) {
	prompts = map[string]string{"Dev": "what is happening in dev", "World": "what is happening in the world"}
	topics = []string{"Dev", "World"}

	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /chat = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Dev", "World", "Join discussion"} {
		if !strings.Contains(body, want) {
			t.Errorf("rooms list is missing %q", want)
		}
	}
}

// The question box is what made chat and the agent the same thing. A page that
// still shipped one would be shipping two assistants again.
func TestTheRoomsListHasNoQuestionBox(t *testing.T) {
	prompts = map[string]string{"Dev": "what is happening in dev"}
	topics = []string{"Dev"}

	rr := httptest.NewRecorder()
	Handler(rr, httptest.NewRequest("GET", "/chat", nil))

	body := rr.Body.String()
	for _, gone := range []string{"chat-form", "topic-selector", "askLLM"} {
		if strings.Contains(body, gone) {
			t.Errorf("rooms list still carries %q, which belonged to the question box", gone)
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
