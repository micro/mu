package inbox

// "Reply goes to …" is for somebody the reader cannot otherwise place.
//
// On /@micro it read "Reply goes to micro@micro.mu" under a conversation with
// @micro — the domain of the server you are signed into, written out as a
// caption under the page you are on.

import (
	"testing"

	"mu/internal/auth"
	"mu/service/mail"
)

func TestOnlyAStrangersReplyAddressIsSpelledOut(t *testing.T) {
	const who = "localreply"
	auth.Create(&auth.Account{ID: who, Name: who}) //nolint:errcheck

	domain := mail.ConfiguredDomain()
	if domain == "" {
		// Nothing is local when no domain is configured, which is itself the
		// right answer: the instance cannot claim an address it has not been
		// given. The bare-handle case below still holds.
		if onThisInstance(who + "@anything.example") {
			t.Error("an address is called local on an instance with no mail domain")
		}
	} else {
		if !onThisInstance(who + "@" + domain) {
			t.Errorf("@%s's own address is not recognised as local", who)
		}
		// A tag is the same account. asim+claude@ is asim.
		if !onThisInstance(who + "+work@" + domain) {
			t.Error("a +tag address is not recognised as the account's own")
		}
		// On this domain and nobody here. This is the half LocalRecipient
		// cannot do on its own: it splits the local part off and would call
		// this local, because it answers "what account would this be" rather
		// than "is there one".
		if onThisInstance("nobodyhere@" + domain) {
			t.Error("an address on this domain with no account behind it is " +
				"treated as somebody on this instance")
		}
	}

	// A bare handle is how this product writes an account, and is what /@name
	// links carry — a local address with the domain left off.
	if !onThisInstance("@" + who) {
		t.Errorf("@%s is not recognised as an account here", who)
	}
	if onThisInstance("@nobodyhere") {
		t.Error("a handle nobody holds is treated as an account here")
	}

	// And the case the line exists for: somebody outside, who may be on several
	// addresses, where which one was chosen is a real fact the reader cannot
	// otherwise see.
	for _, addr := range []string{"henrik@gmail.com", "+447700900000", "", "not-an-address"} {
		if onThisInstance(addr) {
			t.Errorf("%q is treated as an account on this instance, so the "+
				"reply address is hidden for somebody it should be shown for", addr)
		}
	}
}
