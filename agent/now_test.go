package agent

// What goes in front of the question, and what must not.
//
// Each test names a service of its own. The read plane is keyed by service name
// and is process-global — that is the whole point of it, a mirror every reader
// shares — so two tests using one name are two tests sharing a published value,
// and the second reads what the first left behind.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/event"
	"mu/internal/service"
	"mu/internal/world"
)

// A service that says what it already knows gets into the prompt.
func TestWhatIsAlreadyKnownIsInThePrompt(t *testing.T) {
	got := nowContextFrom([]string{"known-svc", "weather"}, saying("known-svc", "Headlines, as of now:\n- [World] Something happened"))
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
	news := saying("scope-svc", "Headlines, as of now:\n- [World] Something happened")
	if got := nowContextFrom([]string{"shell", "apps"}, news); got != "" {
		t.Errorf("an agent that cannot reach news was given the news: %q", got)
	}
	if got := nowContextFrom(nil, news); got != "" {
		t.Errorf("an agent with no tools was given context: %q", got)
	}
}

// A service with nothing to say adds nothing — no heading, no empty section.
func TestSilenceIsSilent(t *testing.T) {
	if got := nowContextFrom([]string{"silent-svc"}, saying("silent-svc", "   ")); got != "" {
		t.Errorf("a service with nothing to say still wrote into the prompt: %q", got)
	}
}

// And a service cannot quietly put a page into every prompt on the instance.
//
// Dropped rather than truncated: half a headline list, read as a complete one,
// is worse than not having it — the model answers "that is all the news there
// is".
func TestAVerboseServiceIsLeftOut(t *testing.T) {
	got := nowContextFrom([]string{"big-svc"}, saying("big-svc", strings.Repeat("x", nowBudget+1)))
	if strings.Contains(got, "xxx") {
		t.Errorf("a Now over the budget went into the prompt: %d characters", len(got))
	}
}

// saying is one service's declaration, with the text it would contribute.
//
// The Now function rather than a published value, which exercises the cold
// start: nothing is on the plane in a test binary, so the block is assembled
// from the declaration and published as it goes. See published.
func saying(name, text string) []service.Spec {
	return []service.Spec{{Name: name, Now: func() string { return text }}}
}

// A service that declares nothing contributes nothing, and does not panic on
// the way — published is called for every spec in the list.
func TestASpecWithNoNowIsSkipped(t *testing.T) {
	if got := nowContextFrom([]string{"quiet-svc"}, []service.Spec{{Name: "quiet-svc"}}); got != "" {
		t.Errorf("a service with no Now wrote into the prompt: %q", got)
	}
}

// What has changed rides along with what is true.
//
// A snapshot says what is true; it cannot say that something happened, and
// "anything new?" is the second question. See internal/world.
func TestWhatChangedIsInThePromptToo(t *testing.T) {
	world.Forget()
	world.Watch()
	event.Announce("delta-svc", "A thing happened", "", "")
	waitForChange(t, "delta-svc")

	got := nowContextFrom([]string{"delta-svc"}, saying("delta-svc", "State, as of now: fine"))
	if !strings.Contains(got, "A thing happened") {
		t.Errorf("the prompt carries the state and not the change: %q", got)
	}
	if !strings.Contains(got, "What has changed") {
		t.Errorf("the change is not labelled as one: %q", got)
	}

	// And it is scoped like everything else: an agent that cannot reach the
	// service does not hear about it.
	if out := nowContextFrom([]string{"shell"}, saying("shell", "")); strings.Contains(out, "A thing happened") {
		t.Errorf("an out-of-scope change reached the prompt: %q", out)
	}
}

func waitForChange(t *testing.T, svc string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(world.Lately(svc)) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no change recorded for %s", svc)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
