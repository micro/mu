package mail

// Mail from your own verified address is never a stranger.
//
// Emailing your own agent is the first thing anyone does with an agent that
// has an address, and it failed twice over: the inbound whitelist was written
// for company domains, so a personal address only got through if an operator
// had added it by hand; and once through, the spam score treated the account
// holder exactly like an unknown sender, so a short message with a link in it
// landed in Filtered where nobody looks.
//
// Verifying an email — clicking a link in the mailbox — is the strongest
// signal this instance has that a person controls an address. Spending it on
// nothing was the bug.

import (
	"os"
	"testing"

	"mu/internal/auth"
)

func TestYourOwnVerifiedAddressSkipsTheSpamFilter(t *testing.T) {
	owner := &auth.Account{ID: "asim", Email: "asim@aslam.me", EmailVerified: true}

	if !isOwnVerifiedAddress(owner, "asim@aslam.me") {
		t.Error("mail from the account's own verified address is being scored as a stranger's")
	}
	// Case and stray whitespace in a header must not defeat it.
	if !isOwnVerifiedAddress(owner, "  ASIM@Aslam.me ") {
		t.Error("a differently-cased From header defeated the check")
	}
	// Somebody else's address is still scored.
	if isOwnVerifiedAddress(owner, "someone@example.com") {
		t.Error("a third party was treated as the account holder")
	}
}

// Unverified is not verified. The whole weight of this rule rests on the
// mailbox having been proven, so an address merely typed into a form earns
// nothing.
func TestAnUnverifiedAddressEarnsNothing(t *testing.T) {
	unverified := &auth.Account{ID: "asim", Email: "asim@aslam.me"}
	if isOwnVerifiedAddress(unverified, "asim@aslam.me") {
		t.Error("an unverified address was trusted")
	}
	if isOwnVerifiedAddress(nil, "asim@aslam.me") {
		t.Error("a nil account was trusted")
	}
	if isOwnVerifiedAddress(&auth.Account{ID: "x", EmailVerified: true}, "") {
		t.Error("an empty address matched an account with no email")
	}
}

// And the inbound filter lets it in without an operator whitelisting the
// domain. aslam.me is not company mail and never will be on a built-in list.
func TestAVerifiedSenderPassesTheInboundFilter(t *testing.T) {
	const addr = "verified-sender-test@aslam.me"
	if _, ok := CheckInboundAllowed(addr, "", ""); ok {
		t.Skip("aslam.me is already allowed by another rule on this instance")
	}

	if err := auth.Create(&auth.Account{ID: "verified-sender-test", Name: "vs", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	acc, err := auth.GetAccount("verified-sender-test")
	if err != nil {
		t.Fatal(err)
	}
	acc.Email, acc.EmailVerified = addr, true
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatal(err)
	}

	if reason, ok := CheckInboundAllowed(addr, "", ""); !ok {
		t.Errorf("a verified account's own address was rejected: %s", reason)
	}
	// An unrelated address on the same domain is still a stranger.
	if _, ok := CheckInboundAllowed("nobody@aslam.me", "", ""); ok {
		t.Error("verifying one address opened the whole domain")
	}
}

// readSource reads a file from this package for the tests that check a rule is
// still expressed in the code rather than only in a comment.
func readSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
