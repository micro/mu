package inbox

import (
	"mu/internal/thread"
	"strings"
	"testing"
	"time"
)

func TestPreviewExcludesLocalChatsBeforeLimiting(t *testing.T) {
	owner := "preview-arrivals"
	received := thread.Open(owner, "mail", "received")
	thread.Name(owner, received.ID, "Incoming message")
	thread.Add(thread.Message{Account: owner, Thread: received.ID, From: "friend@example.test", Text: "Hello", At: time.Now().Add(-time.Hour)})
	for _, client := range []string{thread.WebClient, thread.CLIClient} {
		for _, key := range []string{"one", "two", "three", "four"} {
			c := thread.Open(owner, client, key)
			thread.Name(owner, c.ID, "Local chat")
			thread.Add(thread.Message{Account: owner, Thread: c.ID, Role: thread.RoleAgent, Text: "Reply", At: time.Now()})
		}
	}
	got := Preview(owner)
	if !strings.Contains(got, "Incoming message") || strings.Contains(got, "Local chat") {
		t.Fatal(got)
	}
	only := thread.Open("preview-only-local", thread.WebClient, "local")
	thread.Add(thread.Message{Account: only.Account, Thread: only.ID, Text: "Hello"})
	if Preview(only.Account) != "" {
		t.Fatal("empty incoming preview should be omitted")
	}
}
