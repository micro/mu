package agent

import (
	"strings"
	"testing"
	"time"
)

// A question gets enough tools to finish an ordinary job.
//
// The cap was six, and six is met by work that has gone nothing wrong: read the
// mail, find the invoice, check the date, put it in the calendar is four before
// the model has had a single wrong idea. What was really wanted from that
// number was "do not loop forever", and LoopLimit does that job by name — so
// this one is free to be what it is, a ceiling on what a question may spend.
func TestOneQuestionGetsMoreThanSixTools(t *testing.T) {
	t.Setenv("AGENT_MAX_STEPS", "")
	if n := maxSteps; n < 10 {
		t.Fatalf("maxSteps = %d, too few for work that chains tools", n)
	}
}

// Every path into the native agent carries a deadline.
//
// Both of them ran on context.Background(): ModelCallTimeout bounds one call to
// the provider and nothing bounded the run, so a tool that was merely slow held
// the question open until whatever was upstream gave up. The check is on the
// source because the failure is an absence — a call that was never given a
// context — and a test that ran the agent would need a hanging provider to see
// it.
func TestTheNativeAgentRunsUnderADeadline(t *testing.T) {
	src := readSource(t, "native.go")

	if !strings.Contains(src, "context.WithTimeout(context.Background(), turnTimeout)") {
		t.Error("runNative does not put a deadline on the turn")
	}
	if strings.Contains(src, "a.Ask(context.Background()") {
		t.Error("a.Ask still runs on an unbounded context")
	}
	if strings.Contains(src, "StreamAsk(context.Background()") {
		t.Error("StreamAsk still runs on an unbounded context")
	}
	if turnTimeout < time.Minute {
		t.Errorf("turnTimeout is %s — short enough to cut off honest work", turnTimeout)
	}
}
