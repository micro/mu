package mail

// Finding one message by its id, without reading all of them.
//
// Not the search index — that is index.go, which answers "which of my messages
// mention this" through internal/data. This is the other lookup: the store
// knows exactly which message is wanted and walked the slice to find it.
//
// The store is one flat slice and every lookup walked it. Twenty-five loops
// over `messages` in this package, and the two hot ones are on the delivery
// path: MessageUnlocked resolves a thread root for every new thread, and
// byMessageIDUnlocked resolves a References header for every inbound reply.
// Both are O(n) per delivery, and a delivery holds the write mutex, so the
// whole instance waits for a scan whose length is the size of the mailbox.
//
// It never announces itself. The cost is a millisecond a week, a test store
// never gets big enough to show it, and the instance that does has nothing
// watching. See BenchmarkDeliveryAsTheStoreGrows, which is why this argues
// from measurement rather than from instinct.
//
// # Why the maps cannot go stale
//
// An index maintained by remembering to maintain it is an index that is wrong
// on the path somebody forgot. There were eight places assigning to `messages`
// and one of them — removing a message marked spam — already skipped the inbox
// rebuild that the others did.
//
// So the slice is not assigned directly any more. setMessages replaces it and
// rebuilds; addMessage prepends one and indexes one; both are the only writers,
// and TestNothingAssignsTheStoreDirectly holds that. A stale entry here is
// worse than a slow scan: a lookup that misses reports a message that exists as
// absent, which breaks threading and makes a delete claim there is nothing to
// delete.
//
// # What is not indexed
//
// Per-account lists. The inbox map already answers "what does this account
// have" by thread, which is the shape every reader of that question actually
// wants, and a second per-account index would be a third structure saying the
// same thing — see fileMessage, which maintains the first one.

import "strings"

var (
	// byID is ID -> message. The id is assigned by us and unique.
	byID map[string]*Message

	// byHeaderID is the RFC 5322 Message-ID -> message, lowercased and
	// trimmed because that is how byMessageIDUnlocked compared them.
	//
	// Not unique in principle — a sender can reuse one, and a message
	// delivered to two accounts here is two rows with the same header. First
	// wins, which is what the scan did: it returned the first match, and the
	// slice is newest-first, so "first" meant newest.
	byHeaderID map[string]*Message
)

// setMessages replaces the whole store and rebuilds the lookups.
//
// For load, and for the deletes — anything where the slice is a different
// slice afterwards. Callers must hold the mutex, like everything else here.
func setMessages(msgs []*Message) {
	messages = msgs
	byID = make(map[string]*Message, len(msgs))
	byHeaderID = make(map[string]*Message, len(msgs))
	for _, m := range msgs {
		indexMessageLookups(m)
	}
}

// addMessage puts one message at the front of the store and indexes it.
//
// Newest first, which is the order every reader here assumes.
func addMessage(msg *Message) {
	if msg == nil {
		return
	}
	messages = append([]*Message{msg}, messages...)
	if byID == nil {
		byID = make(map[string]*Message)
	}
	if byHeaderID == nil {
		byHeaderID = make(map[string]*Message)
	}
	indexMessageLookups(msg)
}

// indexMessageLookups records one message under the keys it is looked up by.
//
// The header id is first-wins for the reason on byHeaderID; the id is not,
// because ids are ours and a collision would be a bug worth overwriting into
// view rather than hiding.
func indexMessageLookups(m *Message) {
	if m == nil {
		return
	}
	if m.ID != "" {
		byID[m.ID] = m
	}
	if h := headerKey(m.MessageID); h != "" {
		if _, seen := byHeaderID[h]; !seen {
			byHeaderID[h] = m
		}
	}
}

// headerKey normalises a Message-ID for lookup: trimmed and folded, which is
// what strings.EqualFold did on every comparison in the scan this replaced.
func headerKey(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
