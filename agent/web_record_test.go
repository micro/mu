package agent

// The web writes to the record like every other client.
//
// It was the one that did not, because it streams: it cannot hand the whole
// turn to Ask and wait for an answer. That is a reason to drive its own run,
// not a reason to keep its own record — so it goes through the same three calls
// Ask uses, and there is one implementation of what a conversation is.

import (
	"os"
	"strings"
	"testing"

	"mu/internal/thread"
)

func webSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("agent.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTheWebRecordsWhatWasSaid(t *testing.T) {
	src := webSource(t)
	for _, call := range []string{"Opened(accountID,", "Said(accountID,", "Answered(accountID,"} {
		if !strings.Contains(src, call) {
			t.Errorf("the web does not call %s — it is the only client whose "+
				"conversations are missing from the record", call)
		}
	}
	if !strings.Contains(src, "History(accountID, threadID, historyTurns)") {
		t.Error("the web assembles history from workflow records rather than from " +
			"what was said, so it and every other client remember differently")
	}
}

// One turn, one workflow record.
//
// handleQuery creates the flow and streamNativeSSE finishes it. If the record
// were written by both, every run on the main surface would appear twice —
// visible only as a doubled list, which reads as a display bug rather than a
// data one.
func TestATurnIsRecordedOnce(t *testing.T) {
	src := webSource(t)

	i := strings.Index(src, "func handleQuery(")
	if i < 0 {
		t.Fatal("handleQuery is gone")
	}
	body := src[i:]
	if end := strings.Index(body, "\nfunc "); end > 0 {
		body = body[:end]
	}
	if n := strings.Count(body, "saveFlow("); n != 1 {
		t.Errorf("handleQuery saves %d flows for one turn, want 1", n)
	}
	if n := strings.Count(body, "Said(accountID,"); n != 1 {
		t.Errorf("handleQuery records what was said %d times for one turn, want 1", n)
	}
	if strings.Count(src, "Answered(accountID,") != 1 {
		t.Error("the answer is recorded from more than one place, so a streamed run " +
			"lands in the conversation twice")
	}
}

// No account, nothing to record against. Every run belongs to one now, so this
// is the defensive half: the writes are no-ops rather than panics.
func TestNothingIsRecordedWithoutAnAccount(t *testing.T) {
	if got := Opened("", thread.WebClient, "some-key", "", ""); got != "" {
		t.Errorf("a conversation was opened for an account-less caller (%q)", got)
	}
	// And the writes are no-ops rather than panics when there is no thread.
	Said("", "", "hello", "", "")
	Answered("", "", "hi", "")
	if got := History("", "", 5); got != nil {
		t.Errorf("history for no conversation is %v, want none", got)
	}
}

// The record is what history comes from, and it round-trips.
func TestWhatWasSaidComesBackAsHistory(t *testing.T) {
	const acc = "web-record"
	id := Opened(acc, thread.WebClient, "root-flow-1", "", "")
	if id == "" {
		t.Fatal("no conversation")
	}
	Said(acc, id, "book me a table", "", "")
	Answered(acc, id, "which night?", "flow-1")

	// The same key resolves to the same conversation, which is what makes a
	// second message a continuation rather than a new thread.
	if again := Opened(acc, thread.WebClient, "root-flow-1", "", ""); again != id {
		t.Fatalf("the same conversation opened twice: %q then %q", id, again)
	}

	h := History(acc, id, 10)
	if len(h) != 2 {
		t.Fatalf("history has %d messages, want both sides of one turn: %+v", len(h), h)
	}
	if h[0].Role != "user" || h[0].Text != "book me a table" {
		t.Errorf("first message is %+v", h[0])
	}
	if h[1].Role != "assistant" || h[1].Text != "which night?" {
		t.Errorf("second message is %+v", h[1])
	}
	// And the answer knows which workflow produced it.
	msgs := thread.Messages(acc, id, 0)
	if msgs[1].Workflow != "flow-1" {
		t.Errorf("the answer records workflow %q, want flow-1 — without it there is "+
			"no way back from what was said to how it was produced", msgs[1].Workflow)
	}
}
