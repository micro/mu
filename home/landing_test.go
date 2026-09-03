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

// The corner offers an account, not only a way back to one.
//
// The landing's corner was cut to Log in for the same reason the shell's was —
// see app.headCorner — and this is the page where it cost the most: everybody
// standing here is a stranger, because signing in redirects to /home.
func TestTheLandingOffersAWayToJoin(t *testing.T) {
	got := topRight()
	if !strings.Contains(got, `href="/signup"`) {
		t.Errorf("the front door has no way to sign up on it: %q", got)
	}
	if strings.Index(got, "/signup") > strings.Index(got, "/login") {
		t.Errorf("Log in comes before Sign up on the front door: %q", got)
	}
	// No redirect on the way in. This is the one page where signing in should
	// move you somewhere else, and it already does — carrying the page you were
	// on would be a round trip back to a redirect.
	if strings.Contains(got, "redirect=") {
		t.Errorf("the front door sends you back to itself after signing in: %q", got)
	}
}

// And not where the door is shut.
func TestTheLandingDoesNotOfferSignupOnAnInviteOnlyInstance(t *testing.T) {
	t.Setenv("INVITE_ONLY", "true")
	if got := topRight(); strings.Contains(got, "/signup") {
		t.Errorf("an invite-only instance offers a form nobody can complete: %q", got)
	}
}
