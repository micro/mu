package chat

// What was said over XMPP, written down.
//
// # Why there is no chat store
//
// The first version of this server had no persistence at all: a stanza was
// routed to whatever the recipient had connected and was gone. That is what an
// XMPP server on its own does, and it is wrong here, because this instance
// already has a system of record and the whole argument for one address on two
// protocols is that it is one account underneath. A message that arrives by
// mail is in your inbox forever and a message that arrives by chat vanishing
// would make "one account, reachable two ways" false in the only way that
// matters.
//
// So there is no room file, no chat database and no new store. internal/thread
// is where every client's conversations already go — the web page, the CLI,
// mail — and this is one more client writing to it. A conversation over XMPP
// reads back at /inbox and is searchable by service/recall for the same reason
// theirs are, without either of them learning that XMPP exists.
//
// The websocket rooms in this package are the other thing, and they do keep
// their own file (room_<id>.json, pruned at 24 hours). That is a public room
// attached to an article — many people, no account, disposable by design. A
// one-to-one exchange between two accounts is not that, and giving it the same
// storage would have been storing a conversation in the place built for
// throwing conversations away.
//
// # Offline
//
// A message to somebody who is not connected is still accepted, because it
// lands in their record: they will see it at /inbox whether or not a client was
// running. What they do not yet get is the stanza replayed when they next
// connect — that is XEP-0313 (MAM), a read over exactly this data, and it is the
// next thing rather than a missing foundation.

import (
	"strings"

	"mu/internal/thread"
)

// record writes one exchange into both accounts' records.
//
// Both, because the record is account-scoped: the sender's copy is what they
// sent and the recipient's is what arrived, and neither can read the other's.
// A message to somebody who is not on this instance writes nothing — there is
// no account to write it to, and message() refuses it before this is called.
func record(from, to, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	from, to = strings.ToLower(bareOf(from)), strings.ToLower(bareOf(to))
	key := xmppRoom(from, to)

	// Whose records this belongs in. Both sides normally; one when they are the
	// same account, which is not a silly case — a note to self is how somebody
	// moves a link between their own devices, and it is the case an XMPP client
	// exercises by syncing. Two sides resolving to one account would file it
	// twice in one conversation, so the message would appear duplicated at
	// /inbox with nothing wrong upstream of it.
	//
	// Matched on the account, not the address: you+tag@ and you@ are the same
	// person, so a message from one to the other is still a note to self.
	seen := map[string]bool{}
	for _, side := range [...]struct{ owner, other string }{{from, to}, {to, from}} {
		// The account, not the local part: somebody can add an address to their
		// account, and a conversation filed under the address rather than the
		// owner would be one they cannot read.
		acc := accountFor(side.owner)
		if acc == nil || seen[acc.ID] {
			continue
		}
		seen[acc.ID] = true
		t := thread.Open(acc.ID, thread.ChatClient, key)
		if t == nil {
			continue
		}
		if t.Subject == "" {
			thread.Name(acc.ID, t.ID, side.other)
		}
		// Who is on it. Both sides are people — the agent path does not come
		// through here, it goes to agent/chat and is recorded by agent.Ask.
		thread.Join(acc.ID, t.ID, thread.Party{Kind: thread.RolePerson, Key: from, Name: from})
		thread.Join(acc.ID, t.ID, thread.Party{Kind: thread.RolePerson, Key: to, Name: to})

		thread.Add(thread.Message{
			Thread:  t.ID,
			Account: acc.ID,
			Role:    thread.RolePerson,
			Text:    text,
			From:    from,
			To:      to,
		})
	}
}
