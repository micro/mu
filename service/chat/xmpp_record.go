package chat

// Writing down what went over the wire.
//
// One line of substance, and the interesting part is where it does *not* go.
// This used to write into internal/thread — the record every client's prose
// copy lives in — which made a message between two people, with no agent
// anywhere near it, turn up in an inbox. service/mail has never done that:
// it owns mail and agent/mail writes the prose copy above it. This is the same
// arrangement, arrived at a day late.
//
// See store.go for what it writes to, and agent/chat for the half that reaches
// the record.

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
