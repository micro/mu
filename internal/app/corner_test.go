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
	out := headCorner(nil)
	if !strings.Contains(out, `href="/login"`) {
		t.Errorf("signed out, the corner does not offer a way in: %q", out)
	}
	if !strings.Contains(out, `href="/signup"`) {
		t.Errorf("signed out, the corner does not offer a way to join: %q", out)
	}
	// Sign up first. It is the decision somebody without an account is making;
	// Log in already knows where it is going.
	if strings.Index(out, "/signup") > strings.Index(out, "/login") {
		t.Errorf("Log in comes before Sign up: %q", out)
	}
	// Two links and no third. Install app stood in the old corner and did not
	// earn the slot: it appeared only on some browsers and said nothing about
	// what state you are in or what to do about it, which is the corner's whole
	// job. Browsers offer installing in their own menus.
	if n := strings.Count(out, "<a "); n != 2 {
		t.Errorf("the signed-out corner has %d links, want 2: %q", n, out)
	}
	if strings.Contains(out, "install-app") {
		t.Errorf("Install is back in the corner: %q", out)
	}
}

// And no door onto a form asking for a code.
//
// On an invite-only instance Sign up opens onto "enter your invite code",
// which is a worse answer than not offering it — the person clicking it has no
// code and no way to get one from that page.
func TestNoSignUpWhereSigningUpIsNotAllowed(t *testing.T) {
	t.Setenv("INVITE_ONLY", "true")
	if !auth.InviteOnly() {
		t.Skip("this build does not read INVITE_ONLY, so there is nothing to assert")
	}
	out := headCorner(nil)
	if strings.Contains(out, `href="/signup"`) {
		t.Errorf("an invite-only instance offers Sign up: %q", out)
	}
	if !strings.Contains(out, `href="/login"`) {
		t.Errorf("and then there is no way in at all: %q", out)
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
	got := headCorner(&auth.Account{ID: "tester"})
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
	got := headCorner(&auth.Account{ID: `<script>x</script>`})
	if strings.Contains(got, "<script>") {
		t.Errorf("an account id went into the corner as markup: %q", got)
	}
}
