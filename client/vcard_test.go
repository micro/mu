package client

// The agent as a contact, and what a phone gets when there is nothing to save.

import (
	"strings"
	"testing"

	"mu/internal/settings"
)

func TestACardIsWhatAPhoneReads(t *testing.T) {
	got := VCard("Micro")
	for _, want := range []string{"BEGIN:VCARD", "VERSION:3.0", "FN:Micro", "END:VCARD"} {
		if !strings.Contains(got, want) {
			t.Errorf("the card has no %s:\n%s", want, got)
		}
	}
	// CRLF, which is what the format says and what the stricter readers hold
	// out for. A card with bare newlines imports on a Mac and fails on a phone.
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Error("the card has bare newlines in it")
	}
}

// A name with structure characters in it is a name, not two fields.
//
// An operator names their own agent. A comma or a semicolon in that name splits
// a vCard property in two, and what somebody sees in their phone is half of it.
func TestANameIsNotSplitByItsOwnPunctuation(t *testing.T) {
	got := VCard("Micro, the assistant; really")
	if !strings.Contains(got, `FN:Micro\, the assistant\; really`) {
		t.Errorf("the name was not escaped:\n%s", got)
	}
	// And the escaping is not applied to its own output — backslash first, or
	// every escape is escaped again.
	if strings.Contains(got, `\\,`) {
		t.Errorf("the escape was escaped:\n%s", got)
	}
}

// Nothing configured is not a contact.
//
// Web and CLI are always in All() and neither is something an address book has
// a field for, so a bare instance would offer to save a name and a URL — a
// bookmark with a button promising more than it does.
func TestABareInstanceHasNothingToSave(t *testing.T) {
	if Savable() {
		t.Skip("this box has a number or a mail domain configured")
	}
	// And the door is shut rather than serving an empty card.
	if got := VCard("Micro"); strings.Contains(got, "TEL") || strings.Contains(got, "EMAIL") {
		t.Errorf("a card with no channels carries a number or an address:\n%s", got)
	}
}

// One number is one entry.
//
// The same line often serves SMS and WhatsApp. Two entries for it puts the same
// number in somebody's address book twice, and the second is indistinguishable
// from a mistake — so they are compared by their digits, because +44 7700
// 900000 and +447700900000 are one phone.
func TestTheSameNumberIsNotSavedTwice(t *testing.T) {
	if got := digits("+44 7700 900000"); got != "447700900000" {
		t.Errorf("digits(%q) = %q", "+44 7700 900000", got)
	}
	if !sameNumber("+44 7700 900000", "+447700900000") {
		t.Error("the same number written two ways reads as two numbers")
	}
	if sameNumber("+447700900000", "+447700900001") {
		t.Error("two different numbers read as one")
	}
}

// The card says which instance it is for.
//
// The same agent name on two instances is two assistants with two memories, and
// a phone showing two contacts called Micro with different numbers needs
// something to tell them apart.
func TestTheCardNamesTheInstance(t *testing.T) {
	was := settings.Get("MU_DOMAIN")
	settings.Set("MU_DOMAIN", "example.test")
	t.Cleanup(func() { settings.Set("MU_DOMAIN", was) })

	got := VCard("Micro")
	if !strings.Contains(got, "ORG:example.test") {
		t.Errorf("the card does not say which instance it is for:\n%s", got)
	}
	if !strings.Contains(got, "URL:https://example.test") {
		t.Errorf("the card does not carry the web address:\n%s", got)
	}
}
