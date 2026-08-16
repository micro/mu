package agent

// The mail framing composes with a specialist's own instructions.
//
// Whether a chosen agent survives the router is tested directly in
// routed_test.go; this is the other half — that framing a run for the medium
// does not throw away what the agent is.

import (
	"strings"
	"testing"
)

// And the mail framing survives being composed with a specialist's prompt.
func TestTheMailFramingKeepsTheAgentsOwnInstructions(t *testing.T) {
	markets := Platform("markets")
	if markets == nil {
		t.Fatal("no markets agent")
	}
	base := PlatformOpts(markets).System
	if base == "" {
		t.Fatal("the markets agent has no system prompt to preserve")
	}

	got := MailPrompt(base)
	if !strings.Contains(got, base) {
		t.Error("composing the mail framing dropped the agent's own instructions, so " +
			"a specialist answering mail stops being a specialist")
	}
	if !strings.HasPrefix(got, "You are answering an email") {
		t.Error("the mail framing is not stated first")
	}

	// It has to carry the instructions the observed failures needed.
	for _, want := range []string{"draft", "inbox", "tools"} {
		if !strings.Contains(strings.ToLower(got), want) {
			t.Errorf("the mail framing says nothing about %q — each of these was a "+
				"real reply somebody got instead of an answer", want)
		}
	}

	// With no agent prompt it still frames the run.
	if bare := MailPrompt(""); !strings.HasPrefix(bare, "You are answering an email") ||
		strings.HasSuffix(bare, "\n\n") {
		t.Error("MailPrompt(\"\") is malformed")
	}
}
