package contacts

import (
	"context"
	"os"
	"strings"
	"testing"

	"mu/internal/service"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-contacts-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func clear(owner string) {
	for _, c := range List(owner) {
		_ = Remove(owner, c.ID)
	}
}

// The point of the service: a name becomes an address.
func TestFindTurnsANameIntoAnAddress(t *testing.T) {
	clear("alice")
	defer clear("alice")

	if _, err := Add("alice", "Sarah Chen", "sarah@example.com", "", ""); err != nil {
		t.Fatal(err)
	}

	got := Find("alice", "sarah")
	if len(got) != 1 {
		t.Fatalf("expected one match, got %d", len(got))
	}
	if got[0].Email != "sarah@example.com" {
		t.Errorf("wrong address: %q", got[0].Email)
	}

	// Part of a name, different case, and the address itself all find her.
	for _, q := range []string{"Chen", "SARAH", "sarah@example.com"} {
		if len(Find("alice", q)) != 1 {
			t.Errorf("%q did not find Sarah", q)
		}
	}
}

// An ambiguous name is a question for the person, not something to resolve by
// picking one and sending mail to it.
func TestFindReturnsEveryMatch(t *testing.T) {
	clear("alice")
	defer clear("alice")

	if _, err := Add("alice", "Sarah Chen", "sarah.chen@example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Add("alice", "Sarah Okafor", "sarah.o@example.com", "", ""); err != nil {
		t.Fatal(err)
	}

	if got := Find("alice", "sarah"); len(got) != 2 {
		t.Errorf("an ambiguous name returned %d matches; both should come back", len(got))
	}
}

// Re-adding a known name updates rather than duplicating: an agent told
// "Sarah's new number is X" has no way to know a card already exists.
func TestAddingAKnownNameUpdatesIt(t *testing.T) {
	clear("alice")
	defer clear("alice")

	if _, err := Add("alice", "Sarah Chen", "sarah@example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Add("alice", "sarah chen", "", "+44 7700 900000", "likes early meetings"); err != nil {
		t.Fatal(err)
	}

	got := Find("alice", "sarah")
	if len(got) != 1 {
		t.Fatalf("re-adding made %d cards, want 1", len(got))
	}
	if got[0].Phone == "" {
		t.Error("the new phone number was not saved")
	}
	if got[0].Email != "sarah@example.com" {
		t.Errorf("updating the phone lost the email: %q", got[0].Email)
	}
	if got[0].Note != "likes early meetings" {
		t.Errorf("the note was not saved: %q", got[0].Note)
	}
}

// An address book is personal.
func TestContactsAreScopedToTheirOwner(t *testing.T) {
	clear("alice")
	clear("bob")
	defer clear("alice")

	if _, err := Add("alice", "Sarah Chen", "sarah@example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := Find("bob", "sarah"); len(got) != 0 {
		t.Errorf("bob can see alice's contacts: %v", got)
	}
	if got := List("bob"); len(got) != 0 {
		t.Errorf("bob's address book has alice's contacts: %v", got)
	}
}

func TestBadInputIsRefused(t *testing.T) {
	if _, err := Add("", "Sarah", "", "", ""); err == nil {
		t.Error("a signed-out caller added a contact")
	}
	if _, err := Add("alice", "", "sarah@example.com", "", ""); err == nil {
		t.Error("a contact with no name was accepted")
	}
	if _, err := Add("alice", "Sarah", "not-an-address", "", ""); err == nil {
		t.Error("something that is not an address was accepted as one")
	}
}

// The handler binds the owner from the call context, never a field.
func TestServiceBindsOwnerFromContext(t *testing.T) {
	clear("carol")
	defer clear("carol")

	var add AddResponse
	err := Server{}.Add(service.WithAccount(context.Background(), "carol"),
		&AddRequest{Name: "Dan", Email: "dan@example.com"}, &add)
	if err != nil {
		t.Fatal(err)
	}
	if add.Contact.Owner != "carol" {
		t.Errorf("owner is %q, want carol", add.Contact.Owner)
	}

	if err := (Server{}).List(context.Background(), &ListRequest{}, &ListResponse{}); err == nil {
		t.Error("a caller with no account listed contacts")
	}
}

func TestRenderReadsAsText(t *testing.T) {
	got := Render([]*Contact{{ID: "1", Name: "Sarah Chen", Email: "sarah@example.com", Note: "prefers email"}})
	for _, want := range []string{"Sarah Chen", "<sarah@example.com>", "prefers email"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered contact missing %q: %s", want, got)
		}
	}
	if Render(nil) == "" {
		t.Error("an empty address book rendered nothing at all")
	}
}
