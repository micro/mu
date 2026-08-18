package thread

// Unread: the one piece of state that makes a list of conversations a mailbox.
//
// Without it every row looks the same and the page is a log — you read it top to
// bottom every time, because nothing says which of these you have already dealt
// with. That is the difference between /inbox as a list and /inbox as somewhere
// you work, and it is why it is here rather than on the page: the pages are
// several (the rail's count, the list, the preview on Home) and the answer has
// to be one.
//
// Held as a timestamp on the conversation — see Thread.Seen — so "unread" is a
// comparison rather than a second copy of the truth.

import "time"

// Unread reports whether a conversation has something in it the owner has not
// looked at.
func Unread(t Thread) bool { return t.Updated.After(t.Seen) }

// UnreadCount is how many of an account's conversations are unread.
//
// Walks the account's own threads, which the owned index makes cheap — this is
// drawn on every page, so it must not be a scan of the instance.
func UnreadCount(account string) int {
	mu.RLock()
	defer mu.RUnlock()
	n := 0
	for _, t := range owned[account] {
		if t.Updated.After(t.Seen) {
			n++
		}
	}
	return n
}

// MarkSeen records that the owner has read a conversation up to now.
//
// Called when one is opened. Silent about whether anything changed: the caller
// is a page being rendered, and "it was already read" and "it is read now" are
// the same outcome to it.
func MarkSeen(account, id string) {
	mu.Lock()
	defer mu.Unlock()
	t := threads[id]
	if t == nil || t.Account != account {
		return
	}
	// Now rather than t.Updated: a message that arrives while the page is being
	// drawn is one the reader has not seen, and stamping the conversation's own
	// last-updated time would mark it read.
	t.Seen = time.Now().UTC()
	save()
}

// MarkUnread puts a conversation back to unread.
//
// The other half, and a mailbox without it is one where a mistaken tap loses
// the thing you were keeping. Zeroing Seen rather than subtracting a second
// from Updated, because the question is "have you dealt with this" and the
// honest answer after this call is no.
func MarkUnread(account, id string) {
	mu.Lock()
	defer mu.Unlock()
	t := threads[id]
	if t == nil || t.Account != account {
		return
	}
	t.Seen = time.Time{}
	save()
}
