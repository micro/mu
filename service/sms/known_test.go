package sms

// Who may wake an agent by texting.

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

// The fallback owner files a stranger's text; it does not answer one.
//
// OwnerOf falls back to the operator so nothing is lost, which is right for
// filing and wrong for waking an agent — the fallback is a real account with
// real credits, and every stranger who dialled the number would be talking to
// it. KnownSender is the same lookup without that step.
func TestAStrangerIsFiledButNotAnswered(t *testing.T) {
	const stranger = "+447700900999"

	// OwnerOf answers, because the message has to go somewhere.
	filed := OwnerOf(stranger)
	if filed == "" && auth.Operator() != "" {
		t.Fatal("a stranger's text is not filed anywhere, so this proves nothing")
	}

	// KnownSender does not, because nobody proved this number.
	if owner, known := KnownSender(stranger); known {
		t.Errorf("an unverified number would wake the agent belonging to %q — "+
			"anybody who knows the number could spend that account's credits", owner)
	}
}

// And a number nobody could parse is nobody's.
func TestRubbishIsNotAKnownSender(t *testing.T) {
	for _, n := range []string{"", "   ", "not-a-number", "+"} {
		if owner, known := KnownSender(n); known {
			t.Errorf("%q resolved to %q", n, owner)
		}
	}
}

// Every door normalises, not just the one that remembered.
//
// StartVerify trusted its callers. /sms called e164 first and /account did
// not, so a number typed there arrived raw and countryAllowed refused "0…"
// with "this instance does not send to 0…" — an error about the wrong thing,
// from a page that had done nothing wrong.
func TestStartVerifyNormalisesWhateverItIsGiven(t *testing.T) {
	setup(t)
	t.Setenv("SMS_DEFAULT_COUNTRY", "44")

	// Spaced, punctuated, and national — the three ways a person writes one.
	for _, typed := range []string{"07700 900123", "+44 7700 900123", "07700900123"} {
		err := StartVerify("nobody_has_this_account", typed)
		// It will fail on something later — there is no provider here — but it
		// must not fail on the number itself.
		if err != nil && strings.Contains(err.Error(), "not a phone number") {
			t.Errorf("StartVerify(%q) refused the number: %v", typed, err)
		}
		if err != nil && strings.Contains(err.Error(), "does not send to 0") {
			t.Errorf("StartVerify(%q) saw an unnormalised number: %v", typed, err)
		}
	}
}
