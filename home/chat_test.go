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

func TestGuestChatFirstRunGuidance(t *testing.T) {
	html := chatComponent(true)

	checks := []string{
		`placeholder="Try: give me a morning brief"`,
		"Give me a morning brief",
		"What is moving in markets?",
		`href="/signup"`,
		`href="/login?redirect=/agent"`,
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Fatalf("guest chat HTML missing %q", want)
		}
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

// TestTheLandingPageSaysItOnce — the offer belongs to the page making it, and
// appears there exactly once.
func TestTheLandingPageSaysItOnce(t *testing.T) {
	body := landingBody("https://example.com")
	if n := strings.Count(body, "no account needed"); n != 1 {
		t.Errorf("the landing page makes the guest offer %d times, want 1 — a "+
			"visitor reads the same sentence twice in two stacked paragraphs", n)
	}
}

func TestSignedInChatRendersAsSignedIn(t *testing.T) {
	html := chatComponent(false)
	if !strings.Contains(html, "var GUEST=false") {
		t.Fatalf("signed-in chat should render GUEST=false")
	}
}
