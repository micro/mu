package stream

import (
	"fmt"
	"testing"
	"time"
)

func resetStreamForTest(t *testing.T) {
	t.Helper()
	mu.Lock()
	events = nil
	lastSystemEvent = map[string]time.Time{}
	mu.Unlock()
}

func TestClearResetsSystemThrottle(t *testing.T) {
	resetStreamForTest(t)

	Publish(&Event{Type: TypeSystem, AuthorID: "system", Content: "before clear"})
	if got := len(Recent(10, "")); got != 1 {
		t.Fatalf("after first publish Recent returned %d events, want 1", got)
	}

	Clear()
	Publish(&Event{Type: TypeSystem, AuthorID: "system", Content: "after clear"})

	got := Recent(10, "")
	if len(got) != 1 {
		t.Fatalf("after clear and republish Recent returned %d events, want 1", len(got))
	}
	if got[0].Content != "after clear" {
		t.Fatalf("published event content = %q, want %q", got[0].Content, "after clear")
	}
}

func TestPublishTrimsToMaxEvents(t *testing.T) {
	resetStreamForTest(t)

	for i := range MaxEvents + 5 {
		Publish(&Event{Type: TypeUser, AuthorID: "user", Content: fmt.Sprintf("event-%03d", i)})
	}

	got := Recent(MaxEvents+10, "user")
	if len(got) != MaxEvents {
		t.Fatalf("Recent returned %d events, want %d", len(got), MaxEvents)
	}
	if got[0].Content != "event-504" {
		t.Fatalf("newest content = %q, want event-504", got[0].Content)
	}
	if got[len(got)-1].Content != "event-005" {
		t.Fatalf("oldest retained content = %q, want event-005", got[len(got)-1].Content)
	}
}

func TestContainsMicroRequiresTokenBoundary(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "standalone", text: "hey @micro can you help?", want: true},
		{name: "case insensitive", text: "HEY @MICRO", want: true},
		{name: "punctuation boundary", text: "(@micro): status", want: true},
		{name: "suffix word char", text: "@microservice", want: false},
		{name: "prefix word char", text: "email@micro", want: false},
		{name: "hyphen suffix", text: "@micro-agent", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsMicro(tt.text); got != tt.want {
				t.Fatalf("ContainsMicro(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
