package db

// The store an agent writes to and the store an app writes to are separate.
//
// This was documented the other way round for as long as db_* existed: the
// comment said mu.db and db_* were "literally the same records", so an agent
// could "put something where an app will find it". They are namespaced apart —
// apps get "apps/<slug>" each, this surface gets "api" — and anybody who built
// on the claim would have found their records simply absent, with nothing
// erroring to say why.

import (
	"os"
	"testing"

	"mu/internal/userdb"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-db-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// What an agent stores is not what an app reads, even for the same owner in a
// collection of the same name.
func TestAnAgentsRecordsAreNotAnAppsRecords(t *testing.T) {
	const owner = "asim"

	if _, err := userdb.Create(namespace, owner, "notes", map[string]any{"text": "from the agent"}, false); err != nil {
		t.Fatal(err)
	}

	// The same owner, the same collection name, through an app's namespace.
	appSide, err := userdb.List("apps/notepad", owner, "notes", "mine", nil, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(appSide) != 0 {
		t.Errorf("an app can see %d of the agent's records; the stores are supposed to be separate", len(appSide))
	}

	agentSide, err := userdb.List(namespace, owner, "notes", "mine", nil, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(agentSide) != 1 {
		t.Fatalf("the agent cannot read back its own record: got %d", len(agentSide))
	}
}

// Collections is what the page lists, and it must never show one owner another
// owner's collections — the records are filtered by owner, so a collection with
// nothing of yours in it should not appear at all.
func TestCollectionsAreListedPerOwner(t *testing.T) {
	if _, err := userdb.Create(namespace, "mine", "private-notes", map[string]any{"a": 1}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := userdb.Create(namespace, "theirs", "their-notes", map[string]any{"a": 1}, false); err != nil {
		t.Fatal(err)
	}

	cols, err := userdb.Collections(namespace, "mine")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range cols {
		seen[c.Name] = true
	}
	if !seen["private-notes"] {
		t.Error("an owner cannot see their own collection")
	}
	if seen["their-notes"] {
		t.Error("an owner can see a collection holding only somebody else's records")
	}
}

// A caller with no account has no database.
func TestCollectionsRefusesAnonymous(t *testing.T) {
	if _, err := userdb.Collections(namespace, ""); err == nil {
		t.Error("an unauthenticated caller was given a collection list")
	}
}
