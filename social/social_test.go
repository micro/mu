package social

import (
	"strings"
	"testing"
	"time"
)

func TestThread_ReplyCount(t *testing.T) {
	thread := &Thread{}
	if thread.ReplyCount() != 0 {
		t.Errorf("expected 0 for nil replies, got %d", thread.ReplyCount())
	}

	thread.Replies = []*Reply{
		{ID: "1", Content: "reply 1"},
		{ID: "2", Content: "reply 2"},
	}
	if thread.ReplyCount() != 2 {
		t.Errorf("expected 2, got %d", thread.ReplyCount())
	}
}

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"**bold**", "bold"},
		{"*italic*", "italic"},
		{"# Heading", "Heading"},
		{"## Heading 2", "Heading 2"},
		{"[link](http://example.com)", "link"},
		{"plain text", "plain text"},
		{"- list item", "list item"},
		{"`code`", "code"},
	}
	for _, tt := range tests {
		got := stripMarkdown(tt.input)
		got = strings.TrimSpace(got)
		if got != tt.expected {
			t.Errorf("stripMarkdown(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCommunityNote_Structure(t *testing.T) {
	note := &CommunityNote{
		Content: "This claim is misleading.",
		Sources: []Source{
			{Title: "BBC News", URL: "https://bbc.co.uk/article"},
		},
		Status:    "misleading",
		CheckedAt: time.Now(),
	}
	if note.Content != "This claim is misleading." {
		t.Error("expected content")
	}
	if len(note.Sources) != 1 {
		t.Error("expected 1 source")
	}
	if note.Status != "misleading" {
		t.Error("expected misleading status")
	}
}

func TestThread_Structure(t *testing.T) {
	thread := &Thread{
		ID:        "thread-1",
		Title:     "Discussion",
		Content:   "Some content",
		Topic:     "tech",
		Author:    "Alice",
		AuthorID:  "alice",
		CreatedAt: time.Now(),
		Replies: []*Reply{
			{
				ID:       "reply-1",
				ThreadID: "thread-1",
				Content:  "Great post!",
				Author:   "Bob",
				AuthorID: "bob",
			},
		},
	}
	if thread.ID != "thread-1" {
		t.Error("expected thread ID")
	}
	if thread.ReplyCount() != 1 {
		t.Error("expected 1 reply")
	}
}

func TestReply_Structure(t *testing.T) {
	reply := &Reply{
		ID:       "r1",
		ThreadID: "t1",
		ParentID: "r0",
		Content:  "Nested reply",
		Author:   "Charlie",
		AuthorID: "charlie",
	}
	if reply.ParentID != "r0" {
		t.Error("expected parent ID for nested reply")
	}
}

func TestSource_Structure(t *testing.T) {
	s := Source{
		Title: "Reuters",
		URL:   "https://reuters.com/article",
	}
	if s.Title != "Reuters" {
		t.Error("expected title")
	}
	if s.URL != "https://reuters.com/article" {
		t.Error("expected URL")
	}
}
