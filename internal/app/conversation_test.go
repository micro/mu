package app

import (
	"mu/internal/auth"
	"mu/internal/result"
	"mu/internal/thread"
	"strings"
	"testing"
)

func TestConversationShellHasOneComposerAndOwnedHistory(t *testing.T) {
	const owner = "v2-nav"
	own := thread.Open(owner, thread.WebClient, "owned")
	thread.Add(thread.Message{Account: owner, Thread: own.ID, Text: "Owned conversation"})
	other := thread.Open("v2-other", thread.WebClient, "private")
	thread.Add(thread.Message{Account: "v2-other", Thread: other.ID, Text: "Other private conversation"})
	nav := navMain(&auth.Account{ID: owner})
	if !strings.Contains(nav, own.ID) || strings.Contains(nav, other.ID) {
		t.Fatal("sidebar must contain only owned conversations")
	}
	if !strings.Contains(nav, `href="/bookmarks"`) || !strings.Contains(nav, `href="/services"`) {
		t.Fatal("utilities are missing")
	}
	got := ChatComponent(ChatConfig{Ask: true, ServerOwned: true, StorageNS: "account", ContextID: own.ID})
	if strings.Count(got, `id="mu-chat-input"`) != 1 || strings.Index(got, `id="mu-chat-conv"`) > strings.Index(got, `id="mu-chat-form"`) {
		t.Fatal("transcript must precede one composer")
	}
	if strings.Contains(conversationJS, "mu-console") || strings.Contains(conversationJS, "/assistant") || strings.Contains(conversationJS, "CONTINUE_NS") {
		t.Fatal("retired modes remain")
	}
}

func TestResultRenderingTreatsContentAsData(t *testing.T) {
	got := Results([]result.Item{{Kind: "article", Title: `<img src=x onerror=alert(1)>`, URL: "javascript:alert(1)"}, {Kind: "video", ID: `x\" onload=alert(1)`, Title: "Video"}})
	if strings.Contains(got, "<img") || strings.Contains(got, "javascript:") || strings.Contains(got, "<iframe") {
		t.Fatal("untrusted result became executable content")
	}
	got = Results([]result.Item{{Kind: "video", ID: "abc123", URL: "https://youtube.com/watch?v=abc123", Title: "Arabic fruits"}})
	if !strings.Contains(got, "youtube.com/embed/abc123") || !strings.Contains(got, "data-save-url") {
		t.Fatal("video must play and save in place")
	}
}
