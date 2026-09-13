package work

// Work is done by the agent it was given to.

import (
	"testing"

	"mu/internal/event"
)

// The agent travels with the work.
//
// Nothing carried one: event.RequestWork took an account, a kind, an id, a
// title, a prompt and a thread, so agent/work had no way to know which agent
// had been handed the job and ran the default for every one of them. Handing a
// conversation to a specialist and getting the general agent back is the whole
// feature failing quietly — the answer still arrives, it is just from the wrong
// agent with the wrong tools.
func TestTheAgentTravelsWithTheWork(t *testing.T) {
	done := make(chan request, 1)
	sub := event.Subscribe(event.WorkForAgent)
	defer sub.Close()
	go func() {
		for e := range sub.Chan {
			if r, ok := requestFrom(e.Data); ok {
				done <- r
				return
			}
		}
	}()

	event.RequestWork("acct", "task", "t1", "Book it", "Book it", "thread-9", "research")

	r := <-done
	if r.Agent != "research" {
		t.Errorf("the work arrived for agent %q, want %q — a conversation handed "+
			"to a specialist would be answered by the default agent", r.Agent, "research")
	}
	if r.Thread != "thread-9" {
		t.Errorf("thread = %q, want thread-9", r.Thread)
	}
}

// A task with no agent is the ordinary case and must stay that way.
func TestNoAgentMeansTheDefault(t *testing.T) {
	r, ok := requestFrom(map[string]interface{}{
		"account": "acct", "prompt": "do it",
	})
	if !ok {
		t.Fatal("a request with an account and a prompt is runnable")
	}
	if r.Agent != "" {
		t.Errorf("Agent = %q, want empty so the default answers", r.Agent)
	}
}
