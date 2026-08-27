package sms

// Who may wake an agent by texting.

import (
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
