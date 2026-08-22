package events

// A standing instruction reaching the agent.
//
// These used to set a RunAgent hook and assert on what RunPrompt returned. The
// hook is gone — a service does not run an agent — so what is asserted is the
// fact going on the bus, over the real broker, because a renamed key or an
// unsubscribed topic fails by nothing ever happening, which looks exactly like
// an instruction nobody set.
//
// What the agent does with it, including telling the owner when a run fails, is
// agent/work's and is tested there.

import (
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/event"
)

// asked collects the work requested while f runs.
func asked(t *testing.T, f func()) []map[string]interface{} {
	t.Helper()
	sub := event.Subscribe(event.EventWorkForAgent)
	defer sub.Close()

	f()

	var got []map[string]interface{}
	deadline := time.After(time.Second)
	for {
		select {
		case e := <-sub.Chan:
			got = append(got, e.Data)
			// Nothing here asks for more than one, so the first is the answer;
			// waiting for the timeout on every test costs a second each.
			return got
		case <-deadline:
			return got
		}
	}
}

// A standing instruction is the agent working while nobody is watching, and
// what it is asked is the prompt on the event.
func TestAScheduledInstructionAsksForWork(t *testing.T) {
	auth.Create(&auth.Account{ID: "runner", Name: "runner", Secret: "test-secret"}) //nolint:errcheck

	got := asked(t, func() {
		requestWork(&Event{ID: "e1", Owner: "runner", Title: "Morning brief",
			Prompt: "brief me on the news"})
	})
	if len(got) != 1 {
		t.Fatalf("%d work requests, want one", len(got))
	}
	if got[0]["prompt"] != "brief me on the news" {
		t.Errorf("the agent was asked %q", got[0]["prompt"])
	}
	if got[0]["account"] != "runner" {
		t.Errorf("the work belongs to %q", got[0]["account"])
	}
	// The kind is how a subscriber knows the answer is mailed rather than
	// written back onto a task.
	if got[0]["kind"] != Kind {
		t.Errorf("announced as kind %q, want %q", got[0]["kind"], Kind)
	}
	if got[0]["id"] != "e1" {
		t.Errorf("the request names %q, not the event", got[0]["id"])
	}
}

// Nothing to run is not an error, it is a plain reminder — and OnFire has
// already delivered it.
func TestAnEventWithNoPromptAsksForNothing(t *testing.T) {
	if got := asked(t, func() {
		requestWork(&Event{ID: "e2", Owner: "runner", Title: "Dentist", Prompt: "  "})
	}); len(got) != 0 {
		t.Errorf("a plain reminder asked for work: %v", got)
	}
}

// An instruction belonging to no account never reaches the bus.
//
// Checked because it is a real question and not because of money. It used to
// fall out of the credit check — an unknown account could not be charged, so it
// could not run — and that guard left with the charge when running the agent
// stopped costing credits. A rule that only worked as a side effect of another
// rule is one nobody was holding.
func TestAnInstructionWithNoAccountAsksForNothing(t *testing.T) {
	if got := asked(t, func() {
		requestWork(&Event{ID: "e3", Owner: "nobody-at-all", Title: "Brief", Prompt: "brief me"})
	}); len(got) != 0 {
		t.Errorf("work was requested for an account that does not exist: %v", got)
	}
}
