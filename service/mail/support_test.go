package mail

// support@ has to reach somebody.
//
// "support" is a reserved username, so nobody can hold it — which meant the
// recipient lookup refused it and mail was rejected at RCPT with "User not
// found". An address a product offers, that bounces what is sent to it, is
// worse than no address: somebody who cannot pay writes once and hears nothing,
// and the operator finds out by being told in person.

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

func TestSupportIsAnAddressNobodyCanRegister(t *testing.T) {
	// The premise. If "support" ever becomes registerable, this routing is
	// wrong rather than merely unnecessary — two things would answer for it.
	if reason := auth.ValidateUsername(SupportMailbox); reason == "" {
		t.Error("support is registerable, so it is somebody's account and must " +
			"not also be routed to the admin")
	}
}

func TestSupportMailGoesToTheOldestAdmin(t *testing.T) {
	// Whoever has been operating longest, so adding a second admin later does
	// not silently move an inbox somebody has been watching.
	first := firstAdmin()
	if first == nil {
		t.Skip("no admin on this instance")
	}
	for _, acc := range auth.GetAllAccounts() {
		if acc.Admin && acc.Created.Before(first.Created) {
			t.Errorf("support goes to %s but %s is older", first.ID, acc.ID)
		}
	}
}

func TestSupportAddressIsEmptyWithoutAMailDomain(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "")
	if got := SupportAddress(); got != "" {
		t.Errorf("SupportAddress() = %q on an instance that cannot receive", got)
	}

	t.Setenv("MAIL_DOMAIN", "micro.mu")
	if got := SupportAddress(); got != "support@micro.mu" {
		t.Errorf("SupportAddress() = %q, want support@micro.mu", got)
	}
}

// Both halves of the SMTP path have to know about it: Rcpt accepts the
// recipient, Data resolves it to an account. Either one missing and the mail is
// refused or dropped.
func TestBothHalvesOfDeliveryKnowAboutSupport(t *testing.T) {
	src := readSource(t, "smtp.go")
	if n := strings.Count(src, "SupportMailbox"); n < 2 {
		t.Errorf("only %d place(s) in the SMTP path handle support@ — it has to be "+
			"accepted at RCPT and resolved at DATA, and one without the other is a "+
			"bounce or a silent drop", n)
	}
}
