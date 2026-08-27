package chat

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A room says what it is about, and says it again after a reload.
//
// The summary was computed, carried in roomData and written into the page as
// JSON that nothing read, so a room never showed what it was for. What a reader
// saw was the agent's opening line — generated from this same summary, only
// when the room has had no recent AI message — which is why it appeared on the
// way in and was gone after a refresh.
func TestARoomSaysWhatItIsAbout(t *testing.T) {
	mutex.Lock()
	summaries["testtopic"] = "What people here are arguing about this week."
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		delete(summaries, "testtopic")
		mutex.Unlock()
	}()

	body := func() string {
		rec := httptest.NewRecorder()
		handleGetChat(rec, httptest.NewRequest("GET", "/chat?id=chat_testtopic", nil), "chat_testtopic")
		return rec.Body.String()
	}

	first := body()
	if !strings.Contains(first, "What people here are arguing about this week.") {
		t.Fatal("a room does not say what it is about")
	}
	if !strings.Contains(first, `class="room-about"`) {
		t.Error("the summary is not rendered as the room's own line")
	}

	// The same on the second load, which is the half that was broken: the room
	// exists by now, so nothing is being generated, and the summary has to come
	// from the room itself rather than from a greeting.
	if !strings.Contains(body(), "What people here are arguing about this week.") {
		t.Error("the summary is gone on the second visit")
	}
}
