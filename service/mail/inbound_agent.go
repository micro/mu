package mail

// Who is allowed to wake an agent by writing to it.
//
// An agent's address was protected by nothing but being hard to guess. The
// guard on the trigger asked three questions — is there a tag, is it spam, is
// it our own reply coming back — and never asked who wrote in. Anyone holding
// you+research@ could drive that agent, with its scope and its tools, spending
// the owner's credits. A tag is not a credential.
//
// Two things now have to be true. The sender has to *be* who the From header
// claims, which is what SPF and DKIM are for and which this server has been
// checking all along — the results went into the spam score and nowhere else.
// And the account has to know them: its own verified address, or somebody in
// its address book. A stranger who passes DKIM is still a stranger.
//
// Unknown senders are dropped silently, never bounced. The message is stored
// before any of this runs, so the mail is in the inbox either way; a bounce
// would only confirm the address exists to whoever probed it.

import "strings"

// KnownSender reports whether an address is one this account corresponds with.
// Wired in main.go to the address book, because contacts is a different domain
// and mail should not import it. Nil on an instance without one, which leaves
// the owner's own verified address as the only way in — the safe direction to
// fail.
var KnownSender func(owner, addr string) bool

// shouldWakeAgent is the whole rule, in one place, so it can be read and tested
// without standing up an SMTP session.
//
// authenticated is SPF or DKIM having passed. Either is enough: plenty of
// legitimate mail has only one, and requiring both would drop more real mail
// than it stops.
func shouldWakeAgent(owner, tag, from string, isSpam, authenticated bool) bool {
	if InboundAgent == nil || tag == "" || isSpam {
		return false
	}
	// Our own reply coming back. An agent answering its own answer is a model
	// call per turn, forever.
	if strings.EqualFold(from, "agent@"+GetConfiguredDomain()) {
		return false
	}
	if !authenticated {
		return false
	}
	return senderKnownTo(owner, from)
}

// senderKnownTo: the account's own verified address, or somebody it has in its
// address book. Deliberately narrow — this is the list of people who can spend
// your credits and act as you.
func senderKnownTo(owner, from string) bool {
	if SenderIsAccountOwner(owner, from) {
		return true
	}
	return KnownSender != nil && KnownSender(owner, from)
}
