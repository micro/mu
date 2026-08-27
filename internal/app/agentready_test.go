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

	got := ChatComponent(ChatConfig{Ask: true})

	// It still takes what you type — that is the whole point of not just
	// printing an apology where a box used to be.
	if !strings.Contains(got, `name="q"`) || strings.Contains(got, "<textarea") {
		t.Errorf("the box no longer takes anything:\n%s", got)
	}
	if !strings.Contains(got, `action="/archive"`) {
		t.Errorf("it does not search the archive:\n%s", got)
	}
	if !strings.Contains(got, `method="GET"`) {
		t.Error("a search that is a POST cannot be linked, bookmarked or gone back to")
	}
	// A page that was going to be a chat says why it is not.
	if !strings.Contains(got, "No model is configured") {
		t.Errorf("an agent page with no model does not say why it is a search box:\n%s", got)
	}
	if !strings.Contains(got, "/admin/config") {
		t.Error("it does not say where to add one")
	}
}

// And the ordinary search box explains nothing, because there is nothing to
// explain.
//
// It carried that note unconditionally, from when it rendered only on an
// instance with no model. When search became the default everywhere, every
// instance started telling its owner it had no provider — including the ones
// that do, in production. A note written for one case and reused for all of
// them, asserting something false on most.
func TestAPlainSearchBoxExplainsNothing(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{})
	if strings.Contains(got, "No model is configured") {
		t.Errorf("an instance with a model is being told it has none:\n%s", got)
	}
	// The element, not the class: the stylesheet declares the rule either way,
	// and matching on the class name would fail on the styles rather than on a
	// paragraph anybody can see.
	if strings.Contains(got, `<p class="mu-search-why">`) {
		t.Error("a search box has a paragraph under it explaining that it is a search box")
	}
}

// With a model, it asks — and asserting on more than the form's id, because
// the search box has a form too. "There is a form on the page" is true of both
// and would pass whichever one rendered.
func TestTheAskBoxAsksWhenThereIsAModel(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	// Ask, because search is the default now: a page asks only when it says so.
	got := ChatComponent(ChatConfig{Ask: true})
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
	if got := ChatComponent(ChatConfig{Ask: true}); !strings.Contains(got, "<textarea") {
		t.Error("the ask box vanished because the hook was not wired")
	}
}

// The box is the same box signed in and signed out.
//
// It asked on Home and searched on the index, so the same control did two
// different things depending on whether you had signed in — and once you were,
// there was no consistent way to search anything at all.
//
// Search wins, and not by a coin toss. google.com is a search box and a grid of
// apps; the box is the front door and everything else is reached from beside
// it. That is the shape this already has — a search box and Services — with the
// agent as one of those services rather than the spine. Search also needs
// nothing: no model, no key, no account, so it is the control that works on
// every instance there will ever be.
func TestTheDefaultBoxSearches(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{})
	if !strings.Contains(got, `action="/archive"`) {
		t.Error("a box that was not asked to ask is not searching")
	}
	if strings.Contains(got, "<textarea") {
		t.Error("the default is still a chat box, so Home and the index disagree " +
			"about what the same control does")
	}
}

// And a page that says it is for asking gets an ask box — but only where there
// is something to ask. A model is still the condition; Ask is a statement about
// the page, not a claim that the instance can answer.
func TestAskingStillNeedsAModel(t *testing.T) {
	AgentReady = func() bool { return false }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{Ask: true})
	if strings.Contains(got, "<textarea") {
		t.Error("an agent page offers a chat box on an instance with no model")
	}
	if !strings.Contains(got, `action="/archive"`) {
		t.Error("with no model it does not fall back to the thing that works")
	}
}

// One question, two places to send it.
//
// The box had one arrow and went to /archive, and asking had moved off to the
// agent pages entirely — so somebody on Home with something to ask had to know
// that, leave, and type it again. You often do not know which of the two you
// wanted until you have typed it, so the choice belongs after the typing.
func TestTheBoxOffersSearchAndAskWhenThereIsAModel(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{})

	if !strings.Contains(got, `>Search</button>`) {
		t.Errorf("no Search button:\n%s", got)
	}
	if !strings.Contains(got, `>Ask agent</button>`) {
		t.Errorf("no Ask agent button on an instance that has a model:\n%s", got)
	}
	// The second button sends the same typed question somewhere else. Without
	// formaction it would submit to the form's own action and both buttons
	// would search, which looks exactly like this working.
	if !strings.Contains(got, `formaction="/agent"`) {
		t.Errorf("the Ask button does not go to the agent, so both buttons search:\n%s", got)
	}
	// One field, named the same for both destinations — /archive reads q and
	// /agent reads q. Two inputs, or two names, and one of the buttons submits
	// nothing.
	if n := strings.Count(got, `name="q"`); n != 1 {
		t.Errorf("the box has %d fields named q; both buttons submit the same one", n)
	}
}

// Enter searches.
//
// A form submits through its first submit button, so the order of the two is
// the whole of the keyboard behaviour and nothing else states it. Search first,
// because it is the half that costs nothing and works for everybody — asking
// spends credits and needs an account.
func TestPressingEnterSearchesRatherThanAsks(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{})
	search := strings.Index(got, `>Search</button>`)
	ask := strings.Index(got, `>Ask agent</button>`)
	if search < 0 || ask < 0 {
		t.Fatalf("both buttons are not there:\n%s", got)
	}
	if search > ask {
		t.Error("Ask agent is the first submit button, so Enter asks the agent — " +
			"which spends credits on a keystroke people use to search")
	}
}

// No model, no button. There is nothing behind it, and a control that cannot do
// what it says is worse than one that is not there.
func TestThereIsNoAskButtonWithoutAModel(t *testing.T) {
	AgentReady = func() bool { return false }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{})
	if strings.Contains(got, "Ask agent") || strings.Contains(got, `formaction="/agent"`) {
		t.Errorf("an instance with no model offers to ask the agent:\n%s", got)
	}
	if !strings.Contains(got, `>Search</button>`) {
		t.Error("and the half that works without a model has gone too")
	}
}

// A submit button outside its form submits nothing.
//
// The buttons are drawn under the box rather than inside it, so they are
// siblings of the <form> and not children of it. HTML has an answer for that —
// the form attribute, naming the form the button belongs to — and without it a
// type="submit" button outside a form is inert: it renders, it takes the click,
// the page does not move.
//
// That is exactly how these first shipped, and every test above passed on it:
// they asserted the buttons were present and that one carried formaction, which
// was all true. Driven in a browser, Enter searched (the form submitted itself)
// and neither button did anything at all.
//
// So the assertion is ownership, not presence.
func TestTheButtonsBelongToTheForm(t *testing.T) {
	AgentReady = func() bool { return true }
	t.Cleanup(func() { AgentReady = nil })

	got := ChatComponent(ChatConfig{})
	end := strings.Index(got, "</form>")
	if end < 0 {
		t.Fatal("no form")
	}
	// Every submit button after the form closes has to name the form it is for.
	after := got[end:]
	for _, part := range strings.Split(after, "<button")[1:] {
		btn := part
		if i := strings.Index(btn, ">"); i >= 0 {
			btn = btn[:i]
		}
		if !strings.Contains(btn, `type="submit"`) {
			continue
		}
		if !strings.Contains(btn, `form="mu-search-form"`) {
			t.Errorf("a submit button sits outside the form and does not name it, "+
				"so clicking it does nothing: <button%s>", btn)
		}
	}
}
