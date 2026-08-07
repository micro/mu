package mail

// What the inbox owes somebody who has never used it.
//
// Mu runs a real SMTP server with DKIM, which is most of the reason mail is a
// service here rather than a wrapper around somebody else's. None of that was
// on the page: it showed messages and a compose button, so it was possible to
// use the inbox for a long time without learning you had an address, what it
// was, or why mail sent to it might never arrive.

import (
	"strings"
	"testing"
)

// The address is the first thing an inbox should tell you, and it has to be the
// same one delivery actually resolves — a panel that showed a plausible-looking
// address that did not receive would be worse than showing nothing.
func TestTheInboxShowsTheAddressThatDeliveryResolves(t *testing.T) {
	got := addressPanel("asim")

	want := AliasFor("asim", "")
	if !strings.Contains(got, want) {
		t.Errorf("the panel does not show %q, which is the address that delivers:\n%s", want, got)
	}
}

// An agent with its own address is the thing this service can do that a wrapper
// cannot, and the convention is not guessable — nobody types a plus into an
// address they were never shown.
func TestTheInboxExplainsHowToGiveAnAgentItsOwnAddress(t *testing.T) {
	got := addressPanel("asim")

	tagged := AliasFor("asim", "research")
	if !strings.Contains(got, tagged) {
		t.Errorf("the panel never shows a tagged address like %q:\n%s", tagged, got)
	}
	if !strings.Contains(tagged, "+") {
		t.Fatalf("AliasFor produced %q, which is not a plus address", tagged)
	}
}

// Inbound is deliberately strict: replies, addresses you have written to, and
// known product domains. A filter nobody is told about is indistinguishable
// from mail being broken, and the person who suffers is the one who handed the
// address out expecting it to work.
func TestTheInboxSaysWhatWillActuallyBeDelivered(t *testing.T) {
	got := strings.ToLower(addressPanel("asim"))

	for _, fact := range []string{"replies", "written to"} {
		if !strings.Contains(got, fact) {
			t.Errorf("the panel does not mention %q, so the inbound filter is invisible:\n%s", fact, got)
		}
	}
}

// The address is account-derived, so it must not be somebody else's.
func TestThePanelBelongsToTheAccountItWasAskedAbout(t *testing.T) {
	got := addressPanel("asim")
	if strings.Contains(got, AliasFor("someoneelse", "")) {
		t.Errorf("another account's address appears in the panel:\n%s", got)
	}
}
