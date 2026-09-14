package app

import (
	"strings"
	"testing"

	"mu/internal/thread"
)

func TestConversationHistoryOwnershipAndChannels(t *testing.T) {
	owner := "history-" + t.Name()
	own := thread.Open(owner, thread.WebClient, "own")
	thread.Add(thread.Message{Thread: own.ID, Account: owner, Role: thread.RolePerson, Text: "Find <fruit> videos"})
	other := thread.Open(owner+"-other", thread.WebClient, "other")
	thread.Add(thread.Message{Thread: other.ID, Account: owner + "-other", Text: "Private conversation"})
	incoming := thread.Open(owner, "mail", "incoming")
	thread.Add(thread.Message{Thread: incoming.ID, Account: owner, Text: "Incoming mail"})
	got := ConversationList(owner, own.ID)
	if !strings.Contains(got, "Find &lt;fruit&gt; videos") || !strings.Contains(got, "chat-sess active") || !strings.Contains(got, "<time") {
		t.Fatalf("history is missing its title, current selection or date: %s", got)
	}
	if strings.Contains(got, other.ID) || strings.Contains(got, incoming.ID) {
		t.Fatal("history includes another account or an incoming communication")
	}
	if !strings.Contains(ConversationList(owner+"-empty", ""), "No conversations yet.") {
		t.Fatal("empty history has no explanation")
	}
}
