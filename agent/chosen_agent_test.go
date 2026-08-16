package agent

// An agent somebody chose is the agent that answers.
//
// QueryWithOpts routed on the words in the prompt before looking at whether the
// caller had already picked an agent, and micro.Orchestrate takes no system
// prompt — Execute builds its own from the registry. So a routed run dropped
// opts.System, and with it two things at once: the agent that was addressed,
// and any instruction about the medium.
//
// It showed up in mail. "Tell me more about markets" sent to agent+news@ got
// the markets specialist, because the words beat the address; and the framing
// that tells an agent it is answering an email was discarded for exactly the
// questions that route, which is most of them.

import (
	"strings"
	"testing"
)

// The rule, read off the source: routing is skipped when a system prompt was
// supplied. Running the real thing needs a model, so this pins the branch
// instead — the bug was a missing condition, and a missing condition is visible.
func TestRoutingIsSkippedWhenTheCallerChoseAnAgent(t *testing.T) {
	src := readSource(t, "agent.go")

	i := strings.Index(src, "func QueryWithOpts(")
	if i < 0 {
		t.Fatal("QueryWithOpts is gone")
	}
	body := src[i:]
	if end := strings.Index(body, "\nfunc "); end > 0 {
		body = body[:end]
	}

	route := strings.Index(body, "micro.Route(")
	if route < 0 {
		t.Fatal("QueryWithOpts no longer routes at all")
	}
	guard := strings.Index(body, "opts.System")
	if guard < 0 || guard > route {
		t.Error("QueryWithOpts routes on the prompt before considering whether the " +
			"caller supplied a system prompt — so an addressed agent and its " +
			"instructions are dropped in favour of whatever the words look like")
	}

	// Both routing branches have to be covered, not just the second: direct
	// addressing in the text would otherwise still beat the caller's choice.
	direct := strings.Index(body, "micro.MatchDirectAddress(")
	if direct >= 0 && guard > direct {
		t.Error("direct addressing in the prompt text is handled before the caller's " +
			"own choice of agent is considered")
	}
}

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
