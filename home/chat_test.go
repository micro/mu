package home

// The guest offer is made once.
//
// It was made twice, stacked, in two paragraphs a line apart: the chat
// component wrote a hint under its own input, and the landing page wrote its
// own note under that. Both said a visitor could ask a few questions without an
// account, in almost the same words, one directly above the other.
//
// Neither string was wrong on its own, which is how it survived being edited —
// the copy was changed in both places at once, so both stayed consistent with
// each other and the page went on saying everything twice. What was missing was
// anything that looked at the two together.
//
// The hint is gone. A page that renders a guest chat says what the offer is, in
// its own markup, and the shared component stays a component.

import (
	"strings"
	"testing"
)

// A guest chat offers the two ways on and nothing pre-typed.
//
// It used to require the placeholder "Try: give me a morning brief" and two of
// the four suggestions behind it. Those were the component's defaults, so every
// empty box on the site said the same thing — the landing, Home, the default
// agent, and a specialist's own page, which suggested another agent's work on
// the page you opened to get away from the general one. A suggestion nobody has
// changed in months reads as a demo rather than an offer.
//
// What this holds now is what the guest chat is actually for: a box you can
// type in, and the two ways to keep going once the free queries run out.
func TestGuestChatFirstRunGuidance(t *testing.T) {
	html := chatComponent(true)

	checks := []string{
		`placeholder="Ask it something"`,
		`href="/signup"`,
		`href="/login?redirect=/agent"`,
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Fatalf("guest chat HTML missing %q", want)
		}
	}
	if strings.Contains(html, "morning brief") {
		t.Error("the box still comes pre-loaded with a suggestion nobody wrote for it")
	}
}

// TestTheChatComponentMakesNoOfferOfItsOwn — the component is used on more than
// one page and does not know what any of them are offering. A guest note
// written here is a note the host page cannot remove, which is how the landing
// page ended up with two.
func TestTheChatComponentMakesNoOfferOfItsOwn(t *testing.T) {
	for _, guest := range []bool{true, false} {
		html := chatComponent(guest)
		for _, phrase := range []string{
			"no account needed",
			"questions to try",
			"Sign up when you want",
		} {
			if strings.Contains(html, phrase) {
				t.Errorf("the shared chat component says %q (guest=%v), which the page "+
					"around it also says — that is the duplicate paragraph", phrase, guest)
			}
		}
	}
}

func TestSignedInChatRendersAsSignedIn(t *testing.T) {
	html := chatComponent(false)
	if !strings.Contains(html, "var GUEST=false") {
		t.Fatalf("signed-in chat should render GUEST=false")
	}
}
