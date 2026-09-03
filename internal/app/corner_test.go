package app

// The corner, which is the only thing in the chrome that says which of the two
// states you are in.
//
// This used to be the front door's, in home/index.go, and only the front door
// had one — so a stranger on /contact or /archive had no visible way to sign in
// at all, and the two states of the product differed by which page you were
// standing on rather than by whether you had an account. It moved into the
// shell when the sidebar became collapsed by default, because Login at the foot
// of a hidden rail is not a way in.

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestTheCornerIsTheWayInOrWhatYouHave(t *testing.T) {
	out := headCorner(nil, "")
	if !strings.Contains(out, `href="/login"`) {
		t.Errorf("signed out, the corner does not offer a way in: %q", out)
	}
	// And the way to have an account, which is the other half.
	//
	// This was cut to Log in alone — one control in a corner whose job is to be
	// the one thing you can do from here, with the login page offering signup
	// on it. Signups stopped. A link on the login page is reachable only by
	// somebody who pressed Log in, and a stranger with no account does not
	// press Log in.
	if !strings.Contains(out, `href="/signup"`) {
		t.Errorf("signed out, the corner does not offer a way to have an account: %q", out)
	}
	if n := strings.Count(out, "<a "); n != 2 {
		t.Errorf("the signed-out corner has %d links, want Sign up and Log in: %q", n, out)
	}
	// Sign up first: it is the decision being made here, and Log in already
	// knows where it is going.
	if strings.Index(out, "/signup") > strings.Index(out, "/login") {
		t.Errorf("Log in comes before Sign up in the corner: %q", out)
	}
	// Install app stood here once and appeared on only some browsers, saying
	// nothing about what state you are in or what to do about it — which is the
	// corner's entire purpose.
	if strings.Contains(out, "install-app") {
		t.Errorf("Install is back in the corner: %q", out)
	}
}

// An invite-only instance does not offer a door that is shut.
//
// /signup on one of those is a form asking for a code the person clicking it
// does not have. The corner narrows to Log in there, which is also the width
// difference that was once an argument for cutting Sign up everywhere — kept
// as the exception rather than made the rule.
func TestInviteOnlyDoesNotOfferSignup(t *testing.T) {
	t.Setenv("INVITE_ONLY", "true")
	out := headCorner(nil, "")
	if strings.Contains(out, "/signup") {
		t.Errorf("an invite-only instance offers a signup form nobody can complete: %q", out)
	}
	if !strings.Contains(out, `href="/login"`) {
		t.Errorf("and now there is no way in at all: %q", out)
	}
}

// Signed in, the corner says which account this browser is.
//
// It held only Admin and the balance, both conditional, so an ordinary account
// on an unmetered instance got an empty corner — signed out it said "Sign up ·
// Log in" and signed in it said nothing at all, which is the one question the
// corner exists to answer going unanswered in exactly the state you would check
// it in. That was invisible while the front door drew a corner of its own.
func TestSignedInTheCornerSaysWhoYouAre(t *testing.T) {
	got := headCorner(&auth.Account{ID: "tester"}, "")
	if !strings.Contains(got, "@tester") {
		t.Errorf("the corner does not name the account: %q", got)
	}
	if !strings.Contains(got, `href="/account"`) {
		t.Errorf("the name does not lead anywhere: %q", got)
	}
	// And it is not the signed-out pair.
	if strings.Contains(got, `href="/login"`) || strings.Contains(got, `href="/signup"`) {
		t.Errorf("signed in, the corner still offers a way in: %q", got)
	}
}

// A username is somebody's own text and lands in markup.
func TestTheNameInTheCornerIsEscaped(t *testing.T) {
	got := headCorner(&auth.Account{ID: `<script>x</script>`}, "")
	if strings.Contains(got, "<script>") {
		t.Errorf("an account id went into the corner as markup: %q", got)
	}
}

// Signing in from a page you were reading brings you back to it.
//
// A page that bounced you to /login has always carried a redirect — see
// RedirectToLogin — but a public page you chose to sign in from did not, so
// reading /archive and pressing Log in landed you on /home with the page you
// were on gone.
func TestTheCornerComesBackToThePageYouWereOn(t *testing.T) {
	got := headCorner(nil, "/archive?q=go+micro")
	if !strings.Contains(got, "redirect=%2Farchive%3Fq%3Dgo%2Bmicro") {
		t.Errorf("Log in does not carry the page you were on: %q", got)
	}
	// The landing is the one page where signing in should move you, and it
	// redirects on its own. Carrying it would be a round trip to nowhere.
	if got := headCorner(nil, "/"); strings.Contains(got, "redirect=") {
		t.Errorf("the corner sends you back to the landing: %q", got)
	}
}
