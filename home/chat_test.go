package home

// The shared chat box says nothing the page around it also says.
//
// An offer was once made twice, stacked, in two paragraphs a line apart: the
// component wrote a hint under its own input and the landing page wrote its own
// note under that, both saying a visitor could ask a few questions without an
// account, in almost the same words. Neither string was wrong on its own, which
// is how it survived being edited — the copy was changed in both places at once
// and the page went on saying everything twice.
//
// The hint is gone, and so are signed-out runs. The component stays a
// component.

import (
	"strings"
	"testing"

	"mu/internal/app"
)

// What the box says with nothing typed in it.
//
// It used to require the placeholder "Try: give me a morning brief" and two of
// the four suggestions behind it. Those were the component's defaults, so every
// empty box on the site said the same thing — the landing, Home, the default
// agent, and a specialist's own page, which suggested another agent's work on
// the page you opened to get away from the general one. A suggestion nobody has
// changed in months reads as a demo rather than an offer.
func TestTheChatBoxOffersNoStaleSuggestion(t *testing.T) {
	html := app.ChatComponent(app.ChatConfig{Ask: true})
	if !strings.Contains(html, `placeholder="Ask it something"`) {
		t.Error("the box has no placeholder")
	}
	if strings.Contains(html, "morning brief") {
		t.Error("the box still comes pre-loaded with a suggestion nobody wrote for it")
	}
}

// A session that expired mid-page has somewhere to go.
//
// There were signed-out runs once — three a day, per IP — and this CTA was what
// they ran out into. There are none now, and the 401 still happens: a cookie
// expires while a tab is open, and the answer to a question typed after that is
// a link to the login rather than silence.
func TestAnExpiredSessionIsOfferedTheWayBackIn(t *testing.T) {
	html := app.ChatComponent(app.ChatConfig{Ask: true})
	for _, want := range []string{`href="/signup"`, `href="/login?redirect=/agent"`} {
		if !strings.Contains(html, want) {
			t.Errorf("the chat has no %s for a caller whose session has gone", want)
		}
	}
}

// The component is used on more than one page and does not know what any of
// them are offering. A note written here is a note the host page cannot remove,
// which is how the landing page ended up with two.
func TestTheChatComponentMakesNoOfferOfItsOwn(t *testing.T) {
	html := app.ChatComponent(app.ChatConfig{Ask: true})
	for _, phrase := range []string{
		"no account needed",
		"questions to try",
		"Sign up when you want",
	} {
		if strings.Contains(html, phrase) {
			t.Errorf("the shared chat component says %q, which the page around it "+
				"also says — that is the duplicate paragraph", phrase)
		}
	}
}
