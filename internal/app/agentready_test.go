package app

// With no model the box searches instead of asking.
//
// The model is optional at setup now, and an ask box on an instance without one
// invited a question and then failed on it. Saying "no model yet" was the first
// fix and was only half of one: a box that explains why it is dead is still a
// dead box, at the top of the home screen. The same keystrokes mean something
// without a model — you type what you are looking for, and the difference is
// whether something answers you or finds it.

import (
	"strings"
	"testing"
)

func TestWithNoModelTheBoxSearches(t *testing.T) {
	AgentReady = func() bool { return false }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{})

	// It still takes what you type — that is the whole point of not just
	// printing an apology where a box used to be.
	if !strings.Contains(got, `name="q"`) || strings.Contains(got, "<textarea") {
		t.Errorf("the box no longer takes anything:\n%s", got)
	}
	// At the page that already searches the index, rather than a second one.
	if !strings.Contains(got, `action="/archive"`) {
		t.Errorf("it does not search the archive:\n%s", got)
	}
	if !strings.Contains(got, `method="GET"`) {
		t.Error("a search that is a POST cannot be linked, bookmarked or gone back to")
	}
	// And it says why it is a search rather than a question, so "this is not
	// what I expected" has an answer on the screen.
	if !strings.Contains(got, "no model is configured") {
		t.Errorf("nothing explains why it searches rather than answers:\n%s", got)
	}
	if !strings.Contains(got, "/admin/config") {
		t.Error("it does not say where to add one")
	}
	// The distinction that matters: unconfigured, not broken.
	if !strings.Contains(got, "Everything else works") {
		t.Error("the note does not distinguish unconfigured from broken")
	}
}

// With a model, it asks — and asserting on more than the form's id, because
// the search box has a form too. "There is a form on the page" is true of both
// and would pass whichever one rendered.
func TestTheAskBoxAsksWhenThereIsAModel(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{})
	if !strings.Contains(got, "mu-chat-input") || !strings.Contains(got, "<textarea") {
		t.Error("the ask box is missing on an instance that has a model")
	}
	if strings.Contains(got, `action="/archive"`) {
		t.Error("an instance with a model is being given the search box")
	}
}

// Nil is yes. Everything that renders this component predates the question, and
// a component that turned itself off because nobody wired the hook would be a
// worse bug than the one it fixes.
func TestAnUnwiredHookLeavesTheBoxAlone(t *testing.T) {
	AgentReady = nil
	if got := ChatComponent(ChatConfig{}); !strings.Contains(got, "<textarea") {
		t.Error("the ask box vanished because the hook was not wired")
	}
}
