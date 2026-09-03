package agent

// What goes in front of the question, and what must not.

import (
	"strings"
	"testing"

	"mu/internal/service"
)

// A service that says what it already knows gets into the prompt.
func TestWhatIsAlreadyKnownIsInThePrompt(t *testing.T) {
	got := nowContextFrom([]string{"news", "weather"}, saying("news", "Headlines, as of now:\n- [World] Something happened"))
	if !strings.Contains(got, "Something happened") {
		t.Errorf("the headlines are not in the prompt: %q", got)
	}
	// And the instruction that makes it worth having. Without it the model
	// reads the block and calls the tool anyway, which spends the round trip
	// and the tokens.
	if !strings.Contains(got, "without") || !strings.Contains(got, "tool") {
		t.Errorf("nothing tells the model it may answer from this: %q", got)
	}
}

// Only what this agent could have fetched anyway.
//
// The Code agent is scoped to shell and apps. Putting the news in its prompt is
// paying for a fetch it could not have made, on every question about a file.
func TestAnAgentOutOfScopeGetsNothing(t *testing.T) {
	news := saying("news", "Headlines, as of now:\n- [World] Something happened")
	if got := nowContextFrom([]string{"shell", "apps"}, news); got != "" {
		t.Errorf("an agent that cannot reach news was given the news: %q", got)
	}
	if got := nowContextFrom(nil, news); got != "" {
		t.Errorf("an agent with no tools was given context: %q", got)
	}
}

// A service with nothing to say adds nothing — no heading, no empty section.
func TestSilenceIsSilent(t *testing.T) {
	if got := nowContextFrom([]string{"news"}, saying("news", "   ")); got != "" {
		t.Errorf("a service with nothing to say still wrote into the prompt: %q", got)
	}
}

// And a service cannot quietly put a page into every prompt on the instance.
//
// Dropped rather than truncated: half a headline list, read as a complete one,
// is worse than not having it — the model answers "that is all the news there
// is".
func TestAVerboseServiceIsLeftOut(t *testing.T) {
	got := nowContextFrom([]string{"news"}, saying("news", strings.Repeat("x", nowBudget+1)))
	if strings.Contains(got, "xxx") {
		t.Errorf("a Now over the budget went into the prompt: %d characters", len(got))
	}
}

// saying is one service's declaration, with the text it would contribute.
func saying(name, text string) []service.Spec {
	return []service.Spec{{Name: name, Now: func() string { return text }}}
}
