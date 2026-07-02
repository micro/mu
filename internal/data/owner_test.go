package data

import (
	"os"
	"testing"
)

func ids(entries []*IndexEntry) map[string]bool {
	m := map[string]bool{}
	for _, e := range entries {
		m[e.ID] = true
	}
	return m
}

// TestOwnerScopingInMemory verifies the in-memory index never surfaces one
// account's private entries to an unscoped search or to another account.
func TestOwnerScopingInMemory(t *testing.T) {
	os.Setenv("HOME", t.TempDir())
	UseSQLite = false
	ClearIndex()

	processIndexWork(IndexWork{ID: "pub1", Type: "news", Title: "Public bitcoin rally", Content: "public news about bitcoin"})
	processIndexWork(IndexWork{ID: "a1", Type: "mail", Title: "Alice", Content: "alice private bitcoin mail", Owner: "alice"})
	processIndexWork(IndexWork{ID: "b1", Type: "mail", Title: "Bob", Content: "bob private bitcoin mail", Owner: "bob"})

	// Unscoped search must return ONLY public content.
	got := ids(Search("bitcoin", 10))
	if !got["pub1"] || got["a1"] || got["b1"] {
		t.Errorf("unscoped search leaked private entries: %v", got)
	}

	// Alice-scoped: public + alice, never bob.
	got = ids(Search("bitcoin", 10, WithOwner("alice")))
	if !got["pub1"] || !got["a1"] || got["b1"] {
		t.Errorf("alice-scoped search wrong: %v", got)
	}

	// Bob-scoped: public + bob, never alice.
	got = ids(Search("bitcoin", 10, WithOwner("bob")))
	if !got["pub1"] || !got["b1"] || got["a1"] {
		t.Errorf("bob-scoped search wrong: %v", got)
	}

	// GetByType is public-only.
	if mails := GetByType("mail", 10); len(mails) != 0 {
		t.Errorf("GetByType leaked %d private mail entries", len(mails))
	}
}

// TestOwnerScopingSQLite verifies the same guarantees on the SQLite backend.
func TestOwnerScopingSQLite(t *testing.T) {
	resetSQLiteTestDB(t)

	if err := IndexSQLite("pub1", "news", "Public bitcoin rally", "public news about bitcoin", "", nil); err != nil {
		t.Fatalf("index pub: %v", err)
	}
	if err := IndexSQLite("a1", "mail", "Alice", "alice private bitcoin mail", "alice", nil); err != nil {
		t.Fatalf("index a: %v", err)
	}
	if err := IndexSQLite("b1", "mail", "Bob", "bob private bitcoin mail", "bob", nil); err != nil {
		t.Fatalf("index b: %v", err)
	}

	res, _ := SearchSQLite("bitcoin", 10)
	got := ids(res)
	if !got["pub1"] || got["a1"] || got["b1"] {
		t.Errorf("unscoped SQLite search leaked private entries: %v", got)
	}

	res, _ = SearchSQLite("bitcoin", 10, WithOwner("alice"))
	got = ids(res)
	if !got["pub1"] || !got["a1"] || got["b1"] {
		t.Errorf("alice-scoped SQLite search wrong: %v", got)
	}

	mails, _ := GetByTypeSQLite("mail", 10)
	if len(mails) != 0 {
		t.Errorf("GetByTypeSQLite leaked %d private mail entries", len(mails))
	}
}
