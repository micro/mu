package notes

// Notes are searchable.
//
// They were not. The store is a map of account to notes, held in memory and
// written to memory.json, and the only way to find one was Get by exact title
// or All and read them yourself. So "what did I write down about the flat" was
// not a question this store could answer, and neither was any question that
// crossed into somebody's other material — a note lived nowhere the archive or
// the mail search could see it.
//
// # One entry per note, owned
//
// A note belongs to exactly one account, so unlike mail there is no second
// party to index for. IndexOwned rather than Index, always: an entry with no
// owner is public, and publishing somebody's notes because the wrong function
// was called is the failure this cannot have. See data.Personal.
//
// # The title is the title
//
// The index scores a title above a body, and a note's title is the thing
// somebody chose as its handle — "flat", "Henrik's number". That is exactly
// the word they will search for, so it goes where the scoring can use it.
//
// # Forgetting
//
// Deleting a note unindexes it, and Clear unindexes all of them. An index that
// outlives the thing it describes is worse than no index: it reports a note
// that is gone, and the reader has no way to tell that from one that is there.

import (
	"strings"

	"mu/internal/data"
)

// indexType is what a note is called in the index. The word is data's, like
// every other kind — see data.Vocabulary and data.Personal.
const indexType = data.KindNote

// indexKey is one note, for one account.
//
// The owner is in the key as well as in the row, because a title is unique
// only within an account: two people with a note called "flat" would otherwise
// be one entry, and whichever was written last would be the one both of them
// found.
func indexKey(userID, title string) string {
	return indexType + ":" + userID + ":" + strings.ToLower(strings.TrimSpace(title))
}

// index puts one note where it can be found.
func index(userID string, e *Entry) {
	if e == nil || userID == "" || strings.TrimSpace(e.Title) == "" {
		return
	}
	data.IndexOwned(indexKey(userID, e.Title), indexType,
		e.Title, e.Text, userID, map[string]interface{}{
			"title": e.Title,
		})
}

// unindex forgets one note.
func unindex(userID, title string) {
	if userID == "" || strings.TrimSpace(title) == "" {
		return
	}
	data.Unindex(indexKey(userID, title))
}

// Reindex puts every note on the index.
//
// Run once at boot, in the background, because the index is a separate file
// from the store and an instance that has one and not the other would answer
// every search with nothing. Idempotent — IndexOwned upserts — so running it
// again costs time and changes nothing.
func Reindex() {
	mu.RLock()
	owned := make(map[string][]*Entry, len(store))
	for userID, entries := range store {
		cp := make([]*Entry, len(entries))
		copy(cp, entries)
		owned[userID] = cp
	}
	mu.RUnlock()

	for userID, entries := range owned {
		for _, e := range entries {
			index(userID, e)
		}
	}
}
