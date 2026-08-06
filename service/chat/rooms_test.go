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
	if !strings.Contains(body, "roomData") {
		t.Error("the room page must hand mu.js the room to connect to")
	}
}

// The card is what the home screen shows. Its job is the thing only this
// instance knows, so on a quiet instance it says so rather than rendering an
// empty list.
func TestTheCardSaysWhenNothingIsHappening(t *testing.T) {
	roomsMutex.Lock()
	saved := rooms
	rooms = map[string]*Room{}
	roomsMutex.Unlock()
	defer func() {
		roomsMutex.Lock()
		rooms = saved
		roomsMutex.Unlock()
	}()

	got := Card()
	if !strings.Contains(got, "No discussions") {
		t.Errorf("card on a quiet instance = %q, want it to say nothing is happening", got)
	}
	if !strings.Contains(got, "/chat") {
		t.Error("the card should still offer a way in")
	}
}
