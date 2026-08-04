package auth

import (
	"testing"
	"time"
)

func resetLinkCodes() {
	linkCodeMu.Lock()
	linkCodes = map[string]*linkCode{}
	linkCodeMu.Unlock()
	now = time.Now
}

// The ordinary path: a signed-in user takes a code to a chat channel.
func TestACodeLinksTheAccountItWasIssuedFor(t *testing.T) {
	resetLinkCodes()
	defer resetLinkCodes()

	code := GenerateLinkCode("alice")
	if code == "" {
		t.Fatal("no code was issued")
	}

	got, ok := RedeemLinkCode(code)
	if !ok || got != "alice" {
		t.Errorf("redeemed to %q (ok=%v), want alice", got, ok)
	}
}

// One use. A code left in a chat scrollback must not link a second device.
func TestACodeIsSpentByTheFirstUse(t *testing.T) {
	resetLinkCodes()
	defer resetLinkCodes()

	code := GenerateLinkCode("alice")
	RedeemLinkCode(code)

	if _, ok := RedeemLinkCode(code); ok {
		t.Error("a code was redeemed twice")
	}
}

// Five minutes, and it is worth nothing.
func TestACodeExpires(t *testing.T) {
	resetLinkCodes()
	defer resetLinkCodes()

	at := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	now = func() time.Time { return at }

	code := GenerateLinkCode("alice")
	at = at.Add(LinkCodeTTL + time.Second)

	if _, ok := RedeemLinkCode(code); ok {
		t.Error("an expired code still linked an account")
	}
}

// Issuing again retires the first, so a user who clicks twice does not leave a
// live code behind in the page they navigated away from.
func TestIssuingAgainRetiresTheOldCode(t *testing.T) {
	resetLinkCodes()
	defer resetLinkCodes()

	first := GenerateLinkCode("alice")
	second := GenerateLinkCode("alice")
	if first == second {
		t.Fatal("the same code was issued twice")
	}

	if _, ok := RedeemLinkCode(first); ok {
		t.Error("the superseded code still works")
	}
	if got, ok := RedeemLinkCode(second); !ok || got != "alice" {
		t.Errorf("the current code failed: %q, ok=%v", got, ok)
	}
}

// A guess must not be distinguishable from a miss, and must not link anything.
func TestNonsenseRedeemsToNothing(t *testing.T) {
	resetLinkCodes()
	defer resetLinkCodes()

	GenerateLinkCode("alice")
	for _, s := range []string{"", "   ", "deadbeef", "not-a-code"} {
		if got, ok := RedeemLinkCode(s); ok {
			t.Errorf("RedeemLinkCode(%q) linked %q", s, got)
		}
	}
}

// Codes are hex, so a channel that upper-cases input still works.
func TestRedeemIsCaseAndSpaceInsensitive(t *testing.T) {
	resetLinkCodes()
	defer resetLinkCodes()

	code := GenerateLinkCode("alice")
	if got, ok := RedeemLinkCode("  " + upper(code) + "  "); !ok || got != "alice" {
		t.Errorf("a padded, upper-cased code failed: %q, ok=%v", got, ok)
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

// A signed-out caller has nothing to link.
func TestNoAccountGetsNoCode(t *testing.T) {
	resetLinkCodes()
	defer resetLinkCodes()

	if code := GenerateLinkCode("  "); code != "" {
		t.Errorf("issued %q for no account", code)
	}
}

// Two accounts must not collide.
func TestCodesAreNotSharedBetweenAccounts(t *testing.T) {
	resetLinkCodes()
	defer resetLinkCodes()

	a := GenerateLinkCode("alice")
	b := GenerateLinkCode("bob")

	if got, _ := RedeemLinkCode(a); got != "alice" {
		t.Errorf("alice's code redeemed to %q", got)
	}
	if got, _ := RedeemLinkCode(b); got != "bob" {
		t.Errorf("bob's code redeemed to %q", got)
	}
}
