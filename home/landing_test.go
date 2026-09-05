package home

// What the landing says it is, and how somebody joins from it.

import (
	"strings"
	"testing"
)

// The name is followed by what the thing is.
//
// The wordmark stood alone over a box. "Mu" is three characters of Greek and
// says nothing to somebody who arrived from a link; the <title> and the meta
// description both said personal assistant, so the answer existed on the page
// in two places a reader never looks and in none that they do.
func TestTheWordmarkSaysWhatItIs(t *testing.T) {
	body := indexBody()
	brand := strings.Index(body, `class="lbrand"`)
	what := strings.Index(body, `class="lwhat"`)
	if what < 0 || !strings.Contains(body, "A personal assistant") {
		t.Fatal("the landing names itself and never says what it is")
	}
	// Under the name, not over it: the name is still the name.
	if brand < 0 || what < brand {
		t.Error("the caption is above the wordmark it captions")
	}
	// And quiet. A dark line here is a tagline arguing for something, which is
	// the landing page this one was written to stop being.
	if !strings.Contains(body, ".lwhat{color:#888") {
		t.Error("the caption is not in the page's quiet grey; it reads as a pitch")
	}
}

// The corner offers the public tool catalogue and one way into an account.
//
// Sign up is available from the login page. Keeping it here as a third link
// makes the smallest piece of navigation ask a stranger to choose between two
// account actions before they have decided to use either.
func TestTheLandingOffersToolsAndOneWayIn(t *testing.T) {
	got := topRight()
	for _, want := range []string{`href="/tools"`, `href="/login"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the front door is missing %s: %q", want, got)
		}
	}
	if strings.Contains(got, `href="/signup"`) {
		t.Errorf("the front door has three corner links instead of Tools and Log in: %q", got)
	}
	if strings.Index(got, "/tools") > strings.Index(got, "/login") {
		t.Errorf("Log in comes before Tools on the front door: %q", got)
	}
	// No redirect on the way in. This is the one page where signing in should
	// move you somewhere else, and it already does.
	if strings.Contains(got, "redirect=") {
		t.Errorf("the front door sends you back to itself after signing in: %q", got)
	}
}
