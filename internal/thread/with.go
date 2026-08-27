package thread

// Conversations with somebody, rather than conversations by id.
//
// Every reader over this store has been keyed by conversation: Get takes an id,
// Find takes a client and a key, List takes an account. All three answer "which
// conversation", and none of them answers "what have this person and I said to
// each other" — which is the question somebody asks when they think of a
// correspondent rather than a thread.
//
// It matters because an address is how a thing is reached and how it is named.
// If /@henrik is going to mean the conversation with Henrik, something has to
// be able to find it, and the only thing the store recorded about who is on a
// conversation was Parties — recorded, and until now read by nothing but
// search.
//
// # Exact, not the substring match search uses
//
// Search is looking for anything that might be it and can afford to be
// generous: a hit that is nearly right costs a reader one glance. This decides
// whose conversation a page is, so "sam" matching samantha@ would put one
// person's correspondence on another person's page. Whole values only.

import "strings"

// With returns the account's conversations that any of these keys is a party
// to, newest first.
//
// Keys are whatever names the other side: an address, an account id, a display
// name, a phone number. Several because one person is reachable several ways
// and the caller knows which ones — the account they resolved from a name, the
// address that account receives at — while this does not.
//
// Scoped to the account like every other reader here. Being a party to
// somebody else's conversation grants nothing; see the note on Parties.
func With(account string, keys ...string) []Thread {
	want := make(map[string]bool, len(keys))
	for _, k := range keys {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
			want[k] = true
		}
	}
	if len(want) == 0 {
		return nil
	}

	mu.RLock()
	defer mu.RUnlock()

	var out []Thread
	for _, t := range owned[account] {
		// Held stays out, the same as it does in List: somebody nobody here has
		// heard of does not get to put themselves on a page by writing in.
		if t.Held {
			continue
		}
		if partyIsAnyOf(t, want) {
			out = append(out, *t)
		}
	}
	sortByUpdated(out)
	return out
}

// partyIsAnyOf reports whether one of these names somebody on the conversation.
//
// The thread's own Key too, not only the party list. A conversation from a
// channel that never recorded parties — everything written before Parties
// existed — still knows the address or the number it is with, and dropping
// those would make a person's whole history before some date invisible on their
// page.
func partyIsAnyOf(t *Thread, want map[string]bool) bool {
	if want[strings.ToLower(strings.TrimSpace(t.Key))] {
		return true
	}
	for _, p := range t.Parties {
		if want[strings.ToLower(strings.TrimSpace(p.Key))] ||
			want[strings.ToLower(strings.TrimSpace(p.Name))] {
			return true
		}
	}
	return false
}
