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

import "strings"

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

// AliasFor builds the address an agent should be given.
func AliasFor(account, tag string) string {
	account = strings.TrimSpace(account)
	tag = cleanTag(tag)
	if tag == "" {
		return account + "@" + GetConfiguredDomain()
	}
	return account + "+" + tag + "@" + GetConfiguredDomain()
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
