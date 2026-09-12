package agent

import (
	"mu/internal/thread"
	"strings"
	"testing"
)

func TestRecentConversationsAreOwnStartedChats(t *testing.T) {
	const owner = "recent-preview-owner"
	mine := thread.Open(owner, thread.WebClient, "recent-owned")
	thread.Add(thread.Message{Thread: mine.ID, Account: owner, Text: "My recent question"})
	other := thread.Open("recent-preview-other", thread.WebClient, "recent-other")
	thread.Add(thread.Message{Thread: other.ID, Account: "recent-preview-other", Text: "Private other question"})
	got := RecentConversations(owner)
	if !strings.Contains(got, mine.ID) || !strings.Contains(got, "My recent question") {
		t.Fatal("missing own recent chat")
	}
	if strings.Contains(got, other.ID) || strings.Contains(got, "Private other question") {
		t.Fatal("other account exposed")
	}
	if RecentConversations("") != "" {
		t.Fatal("guest must not have recent chats")
	}
}
