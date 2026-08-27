package app

// An ask box that cannot answer says so before you type in it.
//
// The model is optional at setup now. An instance without one still has mail,
// chat, files, notes and an inbox — but the box on Home invited a question and
// then failed on it, which is the same fault in a different coat: the thing is
// not broken, it is not configured, and only one of those is worth an
// afternoon.

import (
	"strings"
	"testing"
)

func TestTheAskBoxSaysWhenThereIsNoModel(t *testing.T) {
	AgentReady = func() bool { return false }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{})
	if strings.Contains(got, "mu-chat-form") {
		t.Error("a box that cannot answer is still offering to take a question")
	}
	if !strings.Contains(got, "no model") {
		t.Errorf("nothing says why it cannot answer:\n%s", got)
	}
	// And it says the rest of the instance is fine, because "the agent has no
	// model" reads as "this is broken" without it.
	if !strings.Contains(got, "Everything else here works") {
		t.Error("the reason does not distinguish unconfigured from broken")
	}
	if !strings.Contains(got, "/admin/config") {
		t.Error("it does not say where to fix it")
	}
}

// With a model, the box is a box.
func TestTheAskBoxIsABoxWhenThereIsAModel(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	if got := ChatComponent(ChatConfig{}); !strings.Contains(got, "mu-chat-form") {
		t.Error("the ask box is missing on an instance that has a model")
	}
}

// Nil is yes. Everything that renders this component predates the question, and
// a component that turned itself off because nobody wired the hook would be a
// worse bug than the one it fixes.
func TestAnUnwiredHookLeavesTheBoxAlone(t *testing.T) {
	AgentReady = nil
	if got := ChatComponent(ChatConfig{}); !strings.Contains(got, "mu-chat-form") {
		t.Error("the ask box vanished because the hook was not wired")
	}
}
