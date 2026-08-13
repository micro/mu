package mail

// Plus-addressing: an address per agent, without an account per agent.
//
// An agent that can be reached by mail is a different thing from one that can
// only be called: it can be sent a receipt, a form, a reply, a newsletter — and
// woken by any of them. Mu runs a real SMTP server with DKIM, so it is one of
// very few places that can offer this at all.
//
// What it should not do is make an account per agent. Accounts are people:
// they hold credits, own files, sign in. An agent is something a person runs,
// so its address should be a facet of theirs, which is exactly what the
// plus-addressing convention already means to everyone who has used Gmail.
//
// asim+research@micro.mu is asim's inbox, tagged "research". Delivery finds the
// account by the part before the plus; the tag is kept on the message so an
// agent can ask for only its own mail. Nothing else about mail changes: one
// inbox, one set of credits, one reputation to protect.

import (
	"os"
	"strings"
)

// Domain is the mail domain this instance can be reached at, or "" when it has
// none.
//
// Not the same question as GetConfiguredDomain, which answers "localhost" when
// nothing is set so that the SMTP handshake has a hostname to greet with. That
// default is right for a protocol and wrong for everything a person reads, and
// two bugs came out of the difference: an instance with no mail server told an
// agent its address was asim@localhost, and the instance's own readiness check
// asked whether GetConfiguredDomain() was empty, which it never is.
func Domain() string {
	d := strings.ToLower(strings.TrimSpace(os.Getenv("MAIL_DOMAIN")))
	if d == "localhost" {
		return ""
	}
	return d
}

// Reachable reports whether mail here can be reached from outside.
//
// The one thing an SMTP server buys that private messaging does not. Everything
// else about this service — an inbox, a handle, an agent that answers, messages
// between accounts — works with nothing configured at all.
func Reachable() bool { return Domain() != "" }

// Handle is what somebody is called here, with no domain on it: asim, or
// asim+research.
//
// A handle is the whole identity on an instance that is not running a mail
// server, and the local part of the address on one that is. That is the honest
// shape of this service: an inbox that always works, and a domain that turns it
// into an email address when an operator adds one.
func Handle(account, tag string) string {
	account = strings.TrimSpace(account)
	if tag = cleanTag(tag); tag == "" {
		return account
	}
	return account + "+" + tag
}

// SplitAlias separates an address's local part into the account it belongs to
// and the tag after the plus. "asim+research" is ("asim", "research");
// "asim" is ("asim", "").
//
// Only the first plus separates: a tag may itself contain one, so
// "asim+research+daily" tags "research+daily" rather than losing the rest.
func SplitAlias(localPart string) (account, tag string) {
	localPart = strings.TrimSpace(localPart)
	account, tag, found := strings.Cut(localPart, "+")
	if !found {
		return localPart, ""
	}
	// A leading plus has no account before it; treat the whole thing as the
	// account so an address like "+foo@" fails the account lookup rather than
	// resolving to everyone's mail.
	if account == "" {
		return localPart, ""
	}
	return account, tag
}

// AliasFor is what to give somebody so they can reach an agent — an address
// where there is a mail domain, and the handle where there is not.
//
// One function rather than two callers each deciding, because every one of them
// is display: the agent roster, the inbox panel, the address tool. What they
// all want is "what do I show", and the answer is the most a person can
// actually use. It used to append the domain unconditionally, which on an
// unconfigured instance produced asim@localhost — an address that looks real,
// is offered as real, and reaches nobody.
func AliasFor(account, tag string) string {
	h := Handle(account, tag)
	if !Reachable() {
		return h
	}
	return h + "@" + Domain()
}

// CleanTag makes a tag safe to put in an address, so a caller with a name
// rather than a tag — an agent called "Morning Briefer" — can be given one.
func CleanTag(tag string) string { return cleanTag(tag) }

// cleanTag keeps a tag to characters that survive a round trip through every
// mail server between here and the sender. Anything else is dropped rather than
// escaped, because an address that needs quoting is an address that will be
// mistyped.
func cleanTag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	var b strings.Builder
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '.', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// LocalRecipient turns anything that names a mailbox here into the account it
// belongs to: a bare username, a full address, or either with a +tag.
//
// mail_address hands out the full form — asim@micro.mu, asim+claude@micro.mu —
// so a caller writing to what the product told them to write to has to resolve.
func LocalRecipient(to string) string {
	to = strings.ToLower(strings.TrimSpace(to))
	if i := strings.Index(to, "@"); i >= 0 {
		to = to[:i]
	}
	account, _ := SplitAlias(to)
	return account
}
