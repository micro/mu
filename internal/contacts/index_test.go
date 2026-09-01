package contacts

// A contact is findable by anything written on it.
//
// Find matches substrings of the name and the address, and nothing else. So a
// note — the only free text on a contact, and exactly where somebody writes
// "the plumber" — was not searchable at all, and neither was a phone number
// somebody remembered four digits of.

import (
	"strings"
	"testing"

	"mu/internal/data"
)

func TestAContactIsFindableByItsNote(t *testing.T) {
	const who = "ctidx1"
	t.Cleanup(func() { DeleteAll(who) })

	if _, err := Add(who, "Dave Whitmore", "dave@example.com", "07700900461",
		"the plumber, did the boiler in 2024"); err != nil {
		t.Fatalf("add: %v", err)
	}

	// The thing Find cannot see.
	hits := data.Search("plumber", 10, data.WithType(data.KindContact), data.WithOwner(who))
	if len(hits) == 0 {
		t.Fatal("a contact is not findable by its note, which is the only free " +
			"text on it and the reason this index exists")
	}
	if hits[0].Title != "Dave Whitmore" {
		t.Errorf("found %q, want the contact named Dave Whitmore", hits[0].Title)
	}

	// And by a number, which Find also cannot see.
	if h := data.Search("07700900461", 10, data.WithType(data.KindContact),
		data.WithOwner(who)); len(h) == 0 {
		t.Error("a contact is not findable by its phone number")
	}
}

// One account's address book is not another's, and is never public.
func TestAnAddressBookIsNotPublic(t *testing.T) {
	const mine, theirs = "ctidx_mine", "ctidx_theirs"
	t.Cleanup(func() { DeleteAll(mine); DeleteAll(theirs) })

	if _, err := Add(mine, "Priya Raman", "priya@example.com", "", "my accountant"); err != nil {
		t.Fatalf("add: %v", err)
	}

	for _, h := range data.Search("accountant", 10, data.WithType(data.KindContact),
		data.WithOwner(theirs)) {
		if strings.Contains(h.Content, "my accountant") {
			t.Fatal("one account's contact was returned to another")
		}
	}
	// Unowned search is the archive. An address book must never appear in it.
	for _, h := range data.Search("accountant", 10, data.WithType(data.KindContact)) {
		if strings.Contains(h.Content, "my accountant") {
			t.Fatal("a contact came back on an unowned search, so the address " +
				"book is in the archive")
		}
	}
}

// Renaming a contact does not leave the old name behind.
//
// The index is keyed on the record id rather than the name for exactly this: a
// key made of the name would leave a second entry pointing at somebody who is
// not called that any more.
func TestRenamingDoesNotLeaveATwin(t *testing.T) {
	const who = "ctidx_rename"
	t.Cleanup(func() { DeleteAll(who) })

	c, err := Add(who, "Sam Okafor", "sam@example.com", "", "landlord")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	// Same record, new details — Add merges onto an existing exact name.
	if _, err := Add(who, "Sam Okafor", "sam.okafor@example.com", "", "landlord, flat 2"); err != nil {
		t.Fatalf("update: %v", err)
	}

	hits := data.Search("landlord", 20, data.WithType(data.KindContact), data.WithOwner(who))
	if len(hits) != 1 {
		t.Errorf("one contact produced %d index entries — updating made a twin", len(hits))
	}
	if len(hits) > 0 && !strings.Contains(hits[0].Content, "flat 2") {
		t.Errorf("the entry still holds the old note: %q", hits[0].Content)
	}
	_ = c
}

// A removed contact stops being findable.
func TestARemovedContactLeavesTheIndex(t *testing.T) {
	const who = "ctidx_del"
	t.Cleanup(func() { DeleteAll(who) })

	c, err := Add(who, "Temp Person", "", "", "cancel this booking")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if h := data.Search("booking", 10, data.WithType(data.KindContact),
		data.WithOwner(who)); len(h) == 0 {
		t.Fatal("never indexed, so this proves nothing about removing")
	}

	if err := Remove(who, c.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if h := data.Search("booking", 10, data.WithType(data.KindContact),
		data.WithOwner(who)); len(h) > 0 {
		t.Error("a removed contact is still findable — the index outlived it")
	}
}

// And deleting an account takes the whole book.
func TestDeletingAnAccountLeavesNoContactsBehind(t *testing.T) {
	const who = "ctidx_clear"

	for _, n := range []string{"Alpha One", "Bravo Two"} {
		if _, err := Add(who, n, "", "", "keeps the index honest"); err != nil {
			t.Fatalf("add %s: %v", n, err)
		}
	}
	DeleteAll(who)

	if h := data.Search("honest", 10, data.WithType(data.KindContact),
		data.WithOwner(who)); len(h) > 0 {
		t.Errorf("%d contacts are still findable after the account was deleted", len(h))
	}
}
