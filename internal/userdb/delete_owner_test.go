package userdb

// DeleteOwner takes one owner's records and leaves everyone else's alone.
//
// It is the only operation here that runs without a caller to authorise
// against — account deletion happens after the account is gone, so there is
// nobody left to check. Its safety is that it is owner-exact, which is what
// this pins: not "does it delete", but "does it delete only that owner, across
// every collection, and leave the rest intact".

import (
	"os"
	"testing"
)

func store(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mu-userdb-del")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Cleanup(func() { os.RemoveAll(dir) })
}

func TestDeleteOwnerRemovesOneOwnerAcrossEveryCollection(t *testing.T) {
	store(t)

	for _, c := range []struct{ owner, collection, name string }{
		{"alice", "notes", "a1"},
		{"alice", "notes", "a2"},
		{"alice", "recipes", "a3"},
		{"bob", "notes", "b1"},
		{"bob", "recipes", "b2"},
	} {
		if _, err := Create("test", c.owner, c.collection, map[string]any{"n": c.name}, false); err != nil {
			t.Fatal(err)
		}
	}

	n, err := DeleteOwner("test", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("removed %d records, want 3 — it has to reach every collection, not just the first", n)
	}

	for _, collection := range []string{"notes", "recipes"} {
		left, err := List("test", "alice", collection, "mine", nil, "", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(left) != 0 {
			t.Errorf("alice still has %d records in %s", len(left), collection)
		}
	}

	// The point of being owner-exact.
	bobNotes, _ := List("test", "bob", "notes", "mine", nil, "", "", 0)
	bobRecipes, _ := List("test", "bob", "recipes", "mine", nil, "", "", 0)
	if len(bobNotes) != 1 || len(bobRecipes) != 1 {
		t.Errorf("deleting alice took bob's records too: notes=%d recipes=%d",
			len(bobNotes), len(bobRecipes))
	}
}

func TestDeleteOwnerIsSafeToRepeatAndRefusesNonsense(t *testing.T) {
	store(t)
	if _, err := Create("test", "carol", "things", map[string]any{"x": 1}, false); err != nil {
		t.Fatal(err)
	}

	if n, _ := DeleteOwner("test", "carol"); n != 1 {
		t.Errorf("first delete removed %d, want 1", n)
	}
	// Deleting an account twice, or one that stored nothing, is not an error —
	// the hook runs for every account, most of which never touched this store.
	if n, err := DeleteOwner("test", "carol"); err != nil || n != 0 {
		t.Errorf("second delete: n=%d err=%v, want 0 and no error", n, err)
	}
	if n, err := DeleteOwner("test", "never-existed"); err != nil || n != 0 {
		t.Errorf("unknown owner: n=%d err=%v, want 0 and no error", n, err)
	}

	// An empty owner would match records whose owner failed to be set, so it
	// is refused rather than treated as "delete the unowned".
	if _, err := DeleteOwner("test", ""); err == nil {
		t.Error("an empty owner was accepted")
	}
	if _, err := DeleteOwner("../escape", "carol"); err == nil {
		t.Error("a namespace escaping the data directory was accepted")
	}
}
