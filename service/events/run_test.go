package events

import (
	"fmt"
	"strings"
	"testing"

	"mu/internal/auth"
)

// A standing instruction is the agent working while nobody is watching. It is
// the same model call as any other and has to be charged like one — this was
// the one model call the instance gave away.
func TestAScheduledRunCharges(t *testing.T) {
	auth.Create(&auth.Account{ID: "runner", Name: "runner", Secret: "test-secret"})

	asked := ""
	RunAgent = func(accountID, agentID, prompt string) (string, error) {
		asked = prompt
		return "Here is your briefing.", nil
	}
	defer func() { RunAgent = nil }()

	got := RunPrompt(&Event{Owner: "runner", Title: "Morning brief", Prompt: "brief me on the news"})
	if got != "Here is your briefing." {
		t.Errorf("delivered %q", got)
	}
	if asked != "brief me on the news" {
		t.Errorf("the agent was asked %q", asked)
	}
}

// Nothing to run is not an error, it is a plain reminder.
func TestAnEventWithNoPromptRunsNothing(t *testing.T) {
	RunAgent = func(string, string, string) (string, error) {
		t.Error("the agent was called for a plain reminder")
		return "", nil
	}
	defer func() { RunAgent = nil }()

	if got := RunPrompt(&Event{Owner: "runner", Title: "Dentist", Prompt: "  "}); got != "" {
		t.Errorf("a plain reminder produced %q", got)
	}
}

// A run that fails must not be charged, and the owner has to be told: a
// standing instruction that goes quiet looks like it was forgotten.
func TestAFailedRunSaysSoAndIsNotCharged(t *testing.T) {
	auth.Create(&auth.Account{ID: "runner2", Name: "runner2", Secret: "test-secret"})

	RunAgent = func(string, string, string) (string, error) {
		return "", fmt.Errorf("the model is unavailable")
	}
	defer func() { RunAgent = nil }()

	got := RunPrompt(&Event{Owner: "runner2", Title: "Brief", Prompt: "brief me"})
	if !strings.Contains(got, "failed") || !strings.Contains(got, "unavailable") {
		t.Errorf("the owner was told %q", got)
	}
}

// An unknown account cannot be charged, so it cannot run — and says why rather
// than silently doing nothing.
func TestAnUnchargeableRunSaysWhy(t *testing.T) {
	RunAgent = func(string, string, string) (string, error) {
		t.Error("the agent ran for an account that cannot be charged")
		return "", nil
	}
	defer func() { RunAgent = nil }()

	got := RunPrompt(&Event{Owner: "nobody-at-all", Title: "Brief", Prompt: "brief me"})
	if !strings.Contains(got, "did not run") {
		t.Errorf("delivered %q, want an explanation", got)
	}
}

// With no agent wired there is nothing to run, and the owner should hear that
// rather than nothing.
func TestNoAgentIsReported(t *testing.T) {
	RunAgent = nil
	got := RunPrompt(&Event{Owner: "runner", Title: "Brief", Prompt: "brief me"})
	if !strings.Contains(got, "no agent") {
		t.Errorf("delivered %q", got)
	}
}
