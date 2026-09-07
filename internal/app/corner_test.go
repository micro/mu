package app

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestGuestsSignInThroughTheSidebar(t *testing.T) {
	for _, inviteOnly := range []string{"false", "true"} {
		t.Setenv("INVITE_ONLY", inviteOnly)
		if got := headCorner(nil, "/news"); got != "" {
			t.Errorf("guest header duplicates account links: %q", got)
		}
		if got := navMain(nil); got != "" {
			t.Errorf("guest sidebar exposes signed-in destinations: %q", got)
		}
		if got := navBottom(nil); !strings.Contains(got, `href="/login"`) {
			t.Errorf("guest sidebar has no way to log in: %q", got)
		}
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
