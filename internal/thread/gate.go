package thread

// Held: something arrived from somebody this account has never heard of.
//
// # Why the record needs a third state
//
// A conversation was in the record or it was not, and that binary is what made
// an unsolicited text impossible to handle. service/sms dropped one with a log
// line, because the two options were to file it — putting a stranger's words in
// somebody's inbox and waking an agent that costs credits to run — or to lose
// it. Neither is right. Mail has the same problem and solves it earlier, at the
// SMTP door, by refusing strangers outright; that works because a refused
// message bounces back to a sender who finds out. A text has nowhere to bounce
// to, and the number is shared by everybody on the instance.
//
// So: held. It is in the record, it is visible, it is searchable, and nothing
// acts on it. The owner can let it through or leave it. An agent can judge it —
// see agent/gate — which is the whole point of having somewhere for it to sit
// while the judging happens.
//
// # Not the same as unread
//
// Unread is about whether you have looked. Held is about whether it is allowed
// in. A held conversation you have read is still held, and letting one through
// does not mark it read.

import "time"

// Hold marks a conversation as waiting to be let in.
//
// Idempotent, and it does not touch Updated: being held is not something that
// happened on the conversation, it is a fact about how it got here, and bumping
// the timestamp would float every stranger to the top of the list.
func Hold(account, id string) {
	setHeld(account, id, true)
}

// Release lets a held conversation into the inbox proper.
//
// Nothing else changes. It keeps its messages, its parties and its unread mark,
// because it was always in the record — held is a gate, not a quarantine the
// message has to be copied out of.
func Release(account, id string) {
	setHeld(account, id, false)
}

func setHeld(account, id string, held bool) {
	if account == "" || id == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	t := threads[id]
	if t == nil || t.Account != account || t.Held == held {
		return
	}
	t.Held = held
	save()
}

// IsHeld reports whether a conversation is waiting to be let in.
//
// Takes a Thread rather than an id because every caller already has one — a
// page rendering a list would otherwise take the lock once per row.
func IsHeld(t Thread) bool { return t.Held }

// HeldFor is what is waiting to be let in, newest first.
//
// Separate from List rather than a flag on it. List is what the inbox shows and
// held conversations are deliberately not in it: the whole point is that an
// unknown sender cannot put anything in front of you until you or an agent says
// so. A caller that wants both asks for both, which is one line and is honest
// about them being two questions.
func HeldFor(account string, limit int) []Thread {
	mu.RLock()
	defer mu.RUnlock()

	var out []Thread
	for _, t := range owned[account] {
		if t.Held {
			out = append(out, *t)
		}
	}
	sortByUpdated(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// HeldCount is how many are waiting, for a page that wants to say so without
// rendering them.
func HeldCount(account string) int {
	mu.RLock()
	defer mu.RUnlock()
	n := 0
	for _, t := range owned[account] {
		if t.Held {
			n++
		}
	}
	return n
}

// heldSince is how long a conversation may sit held before it stops being worth
// keeping. Nothing expires it yet — this is here to be the number when
// something does, and to say that the decision has been noticed rather than
// missed.
const heldSince = 30 * 24 * time.Hour
