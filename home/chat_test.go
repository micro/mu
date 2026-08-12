package home

import (
	"strings"
	"testing"
)

func TestGuestChatFirstRunGuidance(t *testing.T) {
	html := chatComponent(true)

	checks := []string{
		`placeholder="Try: give me a morning brief"`,
		"A few questions to try it, no account needed.",
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

// The hint lives in one script shared by both, shown or not by a flag. So the
// thing that keeps a signed-in visitor from being told about a guest allowance
// is GUEST=false, not the absence of the string — which is what this checks,
// and what its name has always meant.
func TestSignedInChatDoesNotShowGuestLimitHint(t *testing.T) {
	html := chatComponent(false)
	if !strings.Contains(html, "A few questions to try it, no account needed.") {
		t.Fatalf("shared script should contain guest hint text")
	}
	if !strings.Contains(html, "var GUEST=false") {
		t.Fatalf("signed-in chat should render GUEST=false")
	}
	if !strings.Contains(html, "if(GUEST&&hintDiv)") {
		t.Fatalf("the hint is no longer gated on GUEST, so a signed-in visitor " +
			"is told about an allowance that does not apply to them")
	}
}
