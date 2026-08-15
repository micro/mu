package mail

// Who is allowed to wake an agent by writing to it, and where they can write.
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
//
// There is also one address nobody has to remember. you+research@ requires
// knowing the plus convention and which agent you named what, from a phone,
// while driving. agent@<domain> takes neither: it works out whose it is from
// who sent it, and answers with that account's default agent. It is the
// address agents already reply *from*, so it is also what makes replying to
// your agent continue the conversation rather than bounce.

import (
	"strings"

	"mu/internal/auth"
	"mu/internal/contacts"
)

// AgentMailbox is the local part of the shared address: agent@<domain>. A
// username of the same name is reserved (internal/auth/username.go) so nobody
// can take delivery of it.
const AgentMailbox = "agent"

// SupportMailbox is the local part of support@<domain>.
//
// "support" is a reserved username, so nobody can register it — which meant
// mail to it resolved to no account and was dropped at the door. An address the
// product tells people to write to, that silently discards what they write, is
// worse than not offering one: somebody who cannot top up, or whose payment did
// not land, writes once and hears nothing.
//
// It delivers to the admins, because on this instance they are support.
const SupportMailbox = "support"

// SupportAddress is the address itself, or "" where there is no mail domain.
func SupportAddress() string {
	if d := Domain(); d != "" {
		return SupportMailbox + "@" + d
	}
	return ""
}

// SharedAgentAddress is the address itself, for display and comparison.
func SharedAgentAddress() string {
	domain := ConfiguredDomain()
	if domain == "" {
		return ""
	}
	return AgentMailbox + "@" + domain
}

// KnownSender reports whether an address is one this account corresponds with.
//
// It was a hook the server filled in from service/contacts, on the grounds that
// contacts is a different domain and mail must not import it. That was the
// right rule and the wrong conclusion: the address book itself is
// internal/contacts, and service/contacts is the tools, the page and the Google
// bridge over it. A service may import the substrate freely, so this is an
// import — the sideways rule was never in the way.
//
// It stays a variable because the tests override it, and because an instance
// can still turn the address book off by setting it nil, which leaves the
// owner's own verified address as the only way in — the safe direction to fail.
var KnownSender = contacts.HasEmail

// wakeRequest is everything the rule needs. A struct rather than six
// positional arguments, three of which would be bools in a row.
type wakeRequest struct {
	Owner  string // account the mail is for
	Tag    string // the part after the plus; empty on the shared address
	Shared bool   // arrived at agent@<domain>, so use the default agent
	From   string
	To     string
	IsSpam bool

	// Authenticated is SPF or DKIM having passed. Either is enough: plenty of
	// legitimate mail has only one, and requiring both would drop more real
	// mail than it stops.
	Authenticated bool
}

// mayDispatch is the whole rule, in one place, so it can be read and tested
// without standing up an SMTP session.
//
// It asks whether this message is entitled to wake anything at all — not what
// will be woken, which is the registry's business and used to be a special case
// for agents right here.
func mayDispatch(r wakeRequest) bool {
	if !anyRegistered() || r.IsSpam {
		return false
	}
	// Either a tagged address or the shared one. Untagged mail to your own
	// address is just mail — every newsletter would otherwise start a run.
	if r.Tag == "" && !r.Shared {
		return false
	}
	// Our own reply coming back. An agent answering its own answer is a model
	// call per turn, forever. Two forms: the shared address an agent replies
	// from, and any address writing to itself.
	if strings.EqualFold(r.From, SharedAgentAddress()) {
		return false
	}
	if r.To != "" && strings.EqualFold(r.From, r.To) {
		return false
	}
	if !r.Authenticated {
		return false
	}
	return senderKnownTo(r.Owner, r.From)
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

// firstAdmin is who support mail goes to.
//
// The oldest admin, so a second operator added later does not silently take
// over the inbox somebody has been watching. Nil on an instance with no admin
// at all, which cannot happen after first-run but is not worth crashing over.
func firstAdmin() *auth.Account {
	var out *auth.Account
	for _, acc := range auth.AllAccounts() {
		if !acc.Admin {
			continue
		}
		if out == nil || acc.Created.Before(out.Created) {
			out = acc
		}
	}
	return out
}
