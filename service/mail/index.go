package mail

// Searching a mailbox without reading all of it.
//
// # What this replaces
//
// A loop over every message on the instance. Not every message of the account
// asking — the package-level slice, every account's mail, filtered by owner
// inside the loop and scored with strings.Contains as it went. It was correct
// and it got slower with every message anybody received, which is the shape
// #1464 measured on delivery: 71ms over 5,000 messages, worse than linearly.
//
// internal/data already has an FTS5 index with relevance scoring, and four
// services already use it. Mail was one of the two that did not, and it is the
// one with the most in it.
//
// # One entry per party, not per message
//
// A message here has two sides that can search for it — the recipient, and the
// sender looking through what they sent — and one store holds one copy of it.
// So the index holds one entry per party, keyed by message and owner, because
// the index's own scoping (WithOwner) is what keeps one account's search out of
// another's. Doing it with one entry and filtering afterwards would put the
// filter back in a loop, which is the thing being removed.
//
// # Forgetting
//
// Deleting a message unindexes it. That needed data.Unindex, which did not
// exist: Index and IndexOwned had no opposite, so anything deleted anywhere
// stayed findable. An index that cannot forget is not a cache, it is a second
// copy that outlives the first.

import (
	"strings"

	"mu/internal/data"
)

// indexType is what mail is called in the index.
const indexType = "mail"

// indexKey is one party's view of one message.
//
// The owner is in the key rather than only in the row, so re-indexing a message
// for one party cannot overwrite the other's entry — an upsert keyed on the
// message alone would leave whichever party was written last.
func indexKey(msgID, owner string) string { return indexType + ":" + msgID + ":" + owner }

// indexMessage puts a message in the index for everybody who can search it.
//
// Spam is indexed too, and filtered at the point of reading. A message moved
// out of Junk should be findable without waiting for anything to reindex, and
// the alternative is remembering to index on every path that unmarks one.
func indexMessage(m *Message) {
	if m == nil || m.ID == "" {
		return
	}
	// The searchable text. From is in it because "everything from Stripe" is a
	// search people do, and the subject is separate because the index scores a
	// title above a body.
	body := m.From + " " + m.FromID + " " + m.Body
	for _, owner := range parties(m) {
		data.IndexOwned(indexKey(m.ID, owner), indexType,
			decodeMIMEHeader(m.Subject), body, owner, map[string]interface{}{
				"message": m.ID,
			})
	}
}

// unindexMessage forgets a message, for everybody who could search it.
func unindexMessage(m *Message) {
	if m == nil || m.ID == "" {
		return
	}
	for _, owner := range parties(m) {
		data.Unindex(indexKey(m.ID, owner))
	}
}

// parties is the accounts that can find a message by searching.
//
// The recipient, and the sender when the sender is an account here. An external
// sender has nothing to search with, so there is nothing to index for them.
func parties(m *Message) []string {
	var out []string
	if to := strings.TrimSpace(m.ToID); to != "" {
		out = append(out, to)
	}
	if from := strings.TrimSpace(m.FromID); from != "" && !strings.EqualFold(from, m.ToID) &&
		!IsExternalEmail(from) {
		out = append(out, from)
	}
	return out
}

// Reindex puts the whole mailbox in the index.
//
// Run once at boot, in the background, because the index is a separate file
// from the mailbox and an instance that has one and not the other would answer
// every search with nothing. Idempotent — IndexOwned upserts — so running it
// again costs time and changes nothing.
func Reindex() {
	mutex.RLock()
	all := make([]*Message, len(messages))
	copy(all, messages)
	mutex.RUnlock()

	for _, m := range all {
		indexMessage(m)
	}
}

// searchIndexed is Search over the index.
//
// Asks for more than the caller wants because spam is filtered afterwards and
// a hit may name a message that has since gone — the index is a separate file
// and can be a moment behind the store it describes.
func searchIndexed(userID, query string, limit int) []*Message {
	if userID == "" || strings.TrimSpace(query) == "" {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}
	entries := data.Search(query, limit*3, data.WithType(indexType), data.WithOwner(userID))

	mutex.RLock()
	defer mutex.RUnlock()

	seen := map[string]bool{}
	var out []*Message
	for _, e := range entries {
		id, _ := e.Metadata["message"].(string)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		m := MessageUnlocked(id)
		if m == nil || m.Spam {
			continue
		}
		out = append(out, m)
		if len(out) >= limit {
			break
		}
	}
	return out
}
