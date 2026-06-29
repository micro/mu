package home

import (
	"strings"
	"testing"
)

func TestGuestChatFirstRunGuidance(t *testing.T) {
	html := chatComponent(true)

	checks := []string{
		`placeholder="Try: give me a morning brief"`,
		"No account needed for your first 3 questions.",
		"Give me a morning brief",
		"What is moving in markets?",
		`href="/signup"`,
		`href="/login?redirect=/agent"`,
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Fatalf("guest chat HTML missing %q", want)
		}
	}
}

func TestSignedInChatDoesNotShowGuestLimitHint(t *testing.T) {
	html := chatComponent(false)
	if !strings.Contains(html, "No account needed for your first 3 questions.") {
		t.Fatalf("shared script should contain guest hint text")
	}
	if !strings.Contains(html, "var GUEST=false") {
		t.Fatalf("signed-in chat should render GUEST=false")
	}
}
