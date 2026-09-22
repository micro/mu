package inbox

import (
	"mu/internal/result"
	"mu/internal/thread"
	"strings"
	"testing"
)

func TestAgentMessageKeepsResultCards(t *testing.T) {
	html := messageBlock("owner", &thread.Thread{Client: thread.WebClient}, thread.Message{Role: thread.RoleAgent, Text: "Saved", Results: []result.Item{{Kind: "note", ID: "note-id", Title: "A note", Body: "First\nSecond"}}}, "")
	if !strings.Contains(html, "/notes?id=note-id") || !strings.Contains(html, "First<br>") {
		t.Fatal("Inbox dropped saved result")
	}
}
