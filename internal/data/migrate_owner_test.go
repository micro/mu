package data

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A private entry stays private across the migration.
//
// The migration wrote every entry with an empty owner, on a comment saying
// "pre-owner entries are all public content". That was true while the field did
// not exist and false from the moment IndexOwned was added — and an empty owner
// is what marks an entry public. So switching the search backend would have
// published everything anybody had indexed privately, once, silently, on a
// restart.
//
// It is the kind of bug a migration is uniquely good at: it runs once, on
// somebody else's data, and nobody looks afterwards.
func TestTheMigrationDoesNotPublishPrivateEntries(t *testing.T) {
	// The package-level database handle is shared, so a test that does not
	// reset it migrates into whichever temp directory happened to be first.
	// resetSQLiteTestDB is what the existing migration test uses for the same
	// reason.
	// Both halves matter: without this, Search reads the empty in-memory index
	// and returns nothing — which makes "the private entry is not public" pass
	// for the wrong reason and only the second assertion notices.
	was := UseSQLite
	UseSQLite = true
	t.Cleanup(func() { UseSQLite = was })

	home := resetSQLiteTestDB(t)
	dataDir := filepath.Join(home, ".mu", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}

	old := map[string]*IndexEntry{
		"pub": {ID: "pub", Type: "docs", Title: "A public note",
			Content: "Anybody may read this.", IndexedAt: time.Now()},
		"mine": {ID: "mine", Type: "docs", Title: "My salary review",
			Content: "Confidential.", Owner: "somebody", IndexedAt: time.Now()},
	}
	b, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "index.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MigrateFromJSON(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// The migration is guarded on the table being empty, so a run that skipped
	// would pass everything below without testing anything.
	n, _, err := IndexStats()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("the migration wrote %d entries, want 2 — it skipped, so "+
			"everything below would pass without testing anything", n)
	}

	// A search with no owner sees public content only. If the private entry
	// arrives here, the migration published it.
	for _, e := range Search("salary", 10) {
		if e.ID == "mine" {
			t.Error("a private entry is public after the migration — switching " +
				"the search backend would publish everything anybody had " +
				"indexed privately")
		}
	}
	// And its owner can still find it, so nothing was lost either.
	found := false
	for _, e := range Search("salary", 10, WithOwner("somebody")) {
		if e.ID == "mine" {
			found = true
		}
	}
	if !found {
		t.Error("the owner cannot find their own entry after the migration")
	}
}
