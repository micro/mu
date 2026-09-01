package notes

// A note you wrote is a note you can find.
//
// The store was a map of account to notes with no index at all: Get by exact
// title, or All and read them yourself. So "what did I write down about the
// flat" was not a question it could answer, and a note was invisible to every
// search that crossed into somebody's other material.

import (
	"strings"
	"testing"

	"mu/internal/data"
)

func TestANoteIsFindableByItsWords(t *testing.T) {
	const who = "noteidx1"
	t.Cleanup(func() { Clear(who) })

	Add(who, "Flat", "the boiler service is due in October, engineer is Priya")

	hits := data.Search("boiler", 10, data.WithType(data.KindNote), data.WithOwner(who))
	if len(hits) == 0 {
		t.Fatal("a note is not findable by a word in its body")
	}
	if got := hits[0].Title; got != "Flat" {
		t.Errorf("found %q, want the note titled Flat", got)
	}

	// And by its title, which is the handle somebody chose for it.
	if h := data.Search("Flat", 10, data.WithType(data.KindNote), data.WithOwner(who)); len(h) == 0 {
		t.Error("a note is not findable by its own title")
	}
}

// Somebody else's notes are not yours, whatever you search for.
//
// Every entry here is written with IndexOwned. An entry with no owner is public
// — that is what the archive reads — so writing one of these without an owner
// would publish somebody's notes.
func TestOneAccountsNotesAreNotAnothers(t *testing.T) {
	const mine, theirs = "noteidx_mine", "noteidx_theirs"
	t.Cleanup(func() { Clear(mine); Clear(theirs) })

	Add(mine, "Passport", "renewal booked for March")
	Add(theirs, "Passport", "expires next year")

	hits := data.Search("renewal", 10, data.WithType(data.KindNote), data.WithOwner(theirs))
	for _, h := range hits {
		if strings.Contains(h.Content, "renewal booked") {
			t.Fatal("one account's note was returned to another")
		}
	}

	// And neither is public: the archive must never see a note.
	for _, h := range data.Search("renewal", 10, data.WithType(data.KindNote)) {
		if strings.Contains(h.Content, "renewal booked") {
			t.Fatal("a note came back on an unowned search, so it is in the archive")
		}
	}
}

// Two people can use the same title without becoming one entry.
func TestTheSameTitleForTwoPeopleIsTwoNotes(t *testing.T) {
	const a, b = "noteidx_a", "noteidx_b"
	t.Cleanup(func() { Clear(a); Clear(b) })

	Add(a, "Car", "MOT in June")
	Add(b, "Car", "tyres need replacing")

	ha := data.Search("MOT", 10, data.WithType(data.KindNote), data.WithOwner(a))
	hb := data.Search("tyres", 10, data.WithType(data.KindNote), data.WithOwner(b))
	if len(ha) == 0 || len(hb) == 0 {
		t.Fatalf("one of the two notes titled Car was overwritten by the other "+
			"(a: %d hits, b: %d hits)", len(ha), len(hb))
	}
}

// A rewritten note is findable by what it says now, not by what it said.
func TestRewritingANoteRewritesTheIndex(t *testing.T) {
	const who = "noteidx_rewrite"
	t.Cleanup(func() { Clear(who) })

	Add(who, "Dentist", "appointment on Tuesday")
	Add(who, "Dentist", "moved to Thursday")

	if h := data.Search("Thursday", 10, data.WithType(data.KindNote), data.WithOwner(who)); len(h) == 0 {
		t.Error("the rewritten note is not findable by its new text")
	}
	for _, h := range data.Search("Tuesday", 10, data.WithType(data.KindNote), data.WithOwner(who)) {
		if strings.Contains(h.Content, "appointment on Tuesday") {
			t.Error("the old text of a rewritten note is still findable")
		}
	}
}

// A deleted note stops being findable.
//
// An index that outlives the thing it describes is worse than no index: it
// reports a note that is gone, and the reader cannot tell that from one that
// is there.
func TestADeletedNoteLeavesTheIndex(t *testing.T) {
	const who = "noteidx_del"
	t.Cleanup(func() { Clear(who) })

	Add(who, "Bike", "the lock combination is 4821")
	if h := data.Search("combination", 10, data.WithType(data.KindNote), data.WithOwner(who)); len(h) == 0 {
		t.Fatal("the note was never indexed, so this proves nothing about deleting")
	}

	Delete(who, "Bike")
	if h := data.Search("combination", 10, data.WithType(data.KindNote), data.WithOwner(who)); len(h) > 0 {
		t.Error("a deleted note is still findable — the index outlived the note")
	}
}

// And clearing an account takes all of them.
func TestClearingAnAccountLeavesNoNotesBehind(t *testing.T) {
	const who = "noteidx_clear"

	Add(who, "One", "alpha bravo")
	Add(who, "Two", "charlie delta")
	Clear(who)

	for _, q := range []string{"bravo", "delta"} {
		if h := data.Search(q, 10, data.WithType(data.KindNote), data.WithOwner(who)); len(h) > 0 {
			t.Errorf("%q is still findable after the account was cleared", q)
		}
	}
}
