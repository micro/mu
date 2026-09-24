package chat

// The chat service owns the protocol archive. Its committed records publish
// facts that Inbox can project without involving an agent.

import "strings"

// record keeps one exchange, for each account that was in it.
//
// A copy per account, the way mail keeps one per mailbox: an account's history
// is its own, so deleting yours cannot reach into somebody else's, and two
// people who spoke are two records of having spoken rather than one shared row
// with two owners.
//
// One copy when both sides are the same account — a note to self is how
// somebody moves a link between their own devices, and it is what a client does
// when it syncs. Matched on the account rather than the address, so you+tag@ to
// you@ is still one person.
func record(from, to, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	from, to = strings.ToLower(bareOf(from)), strings.ToLower(bareOf(to))
	conv := xmppRoom(from, to)

	seen := map[string]bool{}
	for _, owner := range [...]string{from, to} {
		acc := accountFor(owner)
		if acc == nil || seen[acc.ID] {
			continue
		}
		seen[acc.ID] = true
		Keep(acc.ID, Said{Conv: conv, From: from, To: to, Text: text})
	}
}
