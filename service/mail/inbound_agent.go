package mail

// Only an account owner may instruct Micro by email. Personal addresses and
// plus aliases receive correspondence; knowing a sender is not authorisation.

import (
	"strings"
)

// AgentMailbox is the local part of the shared address: agent@<domain>. A
// username of the same name is reserved (internal/auth/username.go) so nobody
// can take delivery of it.
const AgentMailbox = "agent"

// SharedAgentAddress is the address itself, for display and comparison.
func SharedAgentAddress() string { return SharedAgentAddressFor("") }

// SharedAgentAddressFor formats a tagged address. Tags classify incoming mail;
// only the untagged shared address may invoke the assistant.
func SharedAgentAddressFor(tag string) string {
	domain := strings.TrimSpace(ConfiguredDomain())
	// localhost is not an address somebody can write to.
	//
	// ConfiguredDomain falls back to it, which is the right answer for an
	// instance talking to itself and the wrong one anywhere a person reads it:
	// a page that prints agent@localhost invites somebody to write to nothing
	// and reads as broken rather than as unconfigured. client.All guarded
	// against exactly this and was the only caller that did — the agent page's
	// empty rail said "write to it at agent+code@localhost" on every
	// development instance.
	//
	// Guarded here rather than at each caller, because which of them remembers
	// is not a property anybody can keep true: this is the one function that
	// knows whether there is a domain, so it is the one that should answer no.
	if domain == "" || strings.EqualFold(domain, "localhost") {
		return ""
	}
	if tag = cleanTag(tag); tag != "" {
		return AgentMailbox + "+" + tag + "@" + domain
	}
	return AgentMailbox + "@" + domain
}

// fromSharedAgent reports whether an address is this instance's own agent
// address, tagged or not.
//
// The loop guard, and it has to see the tag: an agent answering from
// agent+research@ and being written back to would otherwise be a fresh run
// every turn, forever, at a model call each. Comparing against the plain
// address alone stopped being enough the moment the tagged form existed.
func fromSharedAgent(addr string) bool {
	domain := ConfiguredDomain()
	if domain == "" || addr == "" {
		return false
	}
	local, at, found := strings.Cut(addr, "@")
	if !found || !strings.EqualFold(at, domain) {
		return false
	}
	account, _ := SplitAlias(local)
	return strings.EqualFold(account, AgentMailbox)
}

// wakeRequest is everything the rule needs. A struct rather than six
// positional arguments, three of which would be bools in a row.
type wakeRequest struct {
	Owner  string // account the mail is for
	Tag    string // the part after the plus; empty on the shared address
	Shared bool   // arrived at agent@<domain>, so use the default agent
	From   string
	To     string
	IsSpam bool

	// Authenticated means the visible From address passed aligned SPF/DKIM
	// checks, or the sender authenticated through local submission.
	Authenticated bool

	// Owned is the sender having signed in as this account, rather than the
	// From header claiming to be it. Only submission sets it, where a token
	// was presented before the message was accepted. See mayDispatch.
	Owned bool

	// Machine is mail a machine sent without a person asking: a DMARC report,
	// a bounce, a mailing list, an out-of-office. See machineMail.
	Machine bool
}

// mayDispatch is the whole rule, in one place, so it can be read and tested
// without standing up an SMTP session.
//
// It asks whether this message is entitled to wake anything at all — not what
// will be woken, which is the registry's business and used to be a special case
// for agents right here.
func mayDispatch(r wakeRequest) bool {
	if r.IsSpam {
		return false
	}
	// Only agent@ is an instruction endpoint. All plus aliases are filters.
	if !r.Shared || r.Tag != "" {
		return false
	}
	// Our own reply coming back. An agent answering its own answer is a model
	// call per turn, forever. Two forms: the shared address an agent replies
	// from — tagged or not, see fromSharedAgent — and any address writing to
	// itself.
	if fromSharedAgent(r.From) {
		return false
	}
	if r.To != "" && strings.EqualFold(r.From, r.To) {
		return false
	}
	if !r.Authenticated {
		return false
	}
	// Nothing a machine sent on its own. RFC 3834 exists because two automatic
	// responders will talk to each other until somebody notices, and an agent
	// is the most expensive possible participant in that: a model call per
	// turn, forever. A DMARC report is the case that made this obvious — it is
	// DKIM-signed by Google, so it authenticates, and there is nobody on the
	// other end to answer.
	//
	// After the authentication check rather than before, because the headers
	// are a claim like any other and cost nothing to forge — this narrows what
	// authenticated mail may do, it is not a filter on its own.
	if r.Machine {
		return false
	}
	// Signed in as this account, rather than claiming to be it.
	//
	// SenderIsAccountOwner answers "is the From header really this account's owner"
	// for mail arriving off the network, where From is only a claim and a
	// verified external address is the strongest evidence available. Over
	// submission the question was already answered, by a token, before the
	// message was accepted at all — and asking it again returns no, because
	// the From on a submitted message is necessarily the *instance* address
	// (ownsAddress requires it) while a verified address is somebody's
	// external one. So writing to agent@ from a mail client filed the mail and
	// woke nothing, which is the same silence as not having built it.
	if r.Owned {
		return true
	}
	return SenderIsAccountOwner(r.Owner, r.From)
}

// machineMail reports whether a message was sent by a machine on its own
// account, from the headers standardised for exactly this question.
//
// Auto-Submitted is RFC 3834: any value but "no" means automatic, and a
// well-behaved responder must not reply. Precedence is the older convention
// and still what most bulk senders set. List-Id and List-Unsubscribe mark a
// mailing list, which is not a correspondent. X-Auto-Response-Suppress is
// Microsoft's, and worth reading because Exchange is where most of the
// auto-replies in the world come from.
func machineMail(header interface{ Get(string) string }) bool {
	if v := strings.TrimSpace(strings.ToLower(header.Get("Auto-Submitted"))); v != "" && v != "no" {
		return true
	}
	switch strings.TrimSpace(strings.ToLower(header.Get("Precedence"))) {
	case "bulk", "junk", "list", "auto_reply":
		return true
	}
	for _, h := range []string{"List-Id", "List-Unsubscribe", "X-Auto-Response-Suppress", "X-Autoreply", "X-Autorespond"} {
		if strings.TrimSpace(header.Get(h)) != "" {
			return true
		}
	}
	return false
}
