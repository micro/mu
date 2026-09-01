package contacts

// A contact is findable by anything written on it.
//
// Find scans every contact an account has and matches substrings of the name
// and the address. Two consequences, and the second is the one that matters:
// it is a scan, and it does not look at the note or the phone number. So "the
// plumber" — written in a note, which is exactly where somebody puts it — was
// not findable at all, and neither was a number somebody remembered digits of.
//
// Find stays as it is. It answers a different question from search: an agent
// resolving "send this to Sarah" wants everything called Sarah so it can ask
// which, and it wants that whether or not the index has caught up. This is for
// the other question — the one where you know something about a person and not
// their name.
//
// # Owned, always
//
// A contact is one account's, so IndexOwned and never Index. An entry with no
// owner is public, which is what the archive reads, and an address book is the
// last thing that should be in it. See data.Personal.
//
// # What goes in the searchable text
//
// The name is the title, because the index scores a title above a body and a
// name is what a contact is called. Everything else — address, number, note —
// is the body, because each of them is a thing somebody half-remembers and
// searches for. The note especially: it is the only free text on a contact and
// it was the only field Find could not see.

import (
	"strings"

	"mu/internal/auth"
	"mu/internal/data"
)

// indexType is what a contact is called in the index. The word is data's, like
// every other kind — see data.Personal.
const indexType = data.KindContact

// indexKey is one contact, for one account.
//
// Keyed on the record id rather than the name: a contact can be renamed, and a
// key made of the name would leave the old one behind as a second entry
// pointing at a person who is not called that any more.
func indexKey(owner, id string) string { return indexType + ":" + owner + ":" + id }

// index puts one contact where it can be found.
func index(c *Contact) {
	if c == nil || c.Owner == "" || c.ID == "" {
		return
	}
	// Fields joined with a space rather than concatenated, so a search for a
	// number does not match across the boundary between two of them.
	body := strings.TrimSpace(strings.Join([]string{c.Email, c.Phone, c.Note}, " "))
	data.IndexOwned(indexKey(c.Owner, c.ID), indexType,
		c.Name, body, c.Owner, map[string]interface{}{
			"contact": c.ID,
		})
}

// unindex forgets one contact.
func unindex(owner, id string) {
	if owner == "" || id == "" {
		return
	}
	data.Unindex(indexKey(owner, id))
}

// Reindex puts every contact on the index.
//
// Run once at boot, in the background: the index is a separate file from the
// store, so an instance that has one and not the other answers every search
// with nothing — including every instance upgrading to the first build that
// indexes contacts at all. Idempotent, because IndexOwned upserts.
//
// Driven from the account list rather than from the store, because userdb
// cannot enumerate owners — it can create, read, update, delete and delete an
// owner's records, and there is no "who has records here". Walking the accounts
// asks the same question from the side that can answer it, and an account with
// no contacts costs one empty List.
func Reindex() {
	for _, acc := range auth.AllAccounts() {
		if acc == nil || acc.ID == "" {
			continue
		}
		for _, c := range List(acc.ID) {
			index(c)
		}
	}
}
