package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Which index is live has to be readable from the running instance.
//
// Two implementations leave two files — a 32MB index.json and a 366MB index.db
// on the same disk — and from outside they look the same. The failure modes are
// opposite: a hot JSON index is a write to remove, a dead one is a file to
// delete. Guessing between them is how a whole-file rewrite per crawled article
// stays invisible.
func TestTheLiveIndexSaysWhichItIs(t *testing.T) {
	was := UseSQLite
	t.Cleanup(func() { UseSQLite = was })

	UseSQLite = true
	if got := SearchBackend(); !strings.Contains(got, "index.db") {
		t.Errorf("with SQLite on, the backend reads %q", got)
	}
	if stale := Stale(); !contains(stale, "index.json") {
		t.Errorf("with SQLite on, index.json is not named as unwritten: %v", stale)
	}

	UseSQLite = false
	if got := SearchBackend(); !strings.Contains(got, "index.json") {
		t.Errorf("with SQLite off, the backend reads %q", got)
	}
	if stale := Stale(); !contains(stale, "index.db") {
		t.Errorf("with SQLite off, index.db is not named as unwritten: %v", stale)
	}
}

// And the cost is said out loud where it applies, because "rewritten whole" is
// the fact that turns 32MB from a size into a problem.
func TestTheJSONIndexSaysWhatItCosts(t *testing.T) {
	was := UseSQLite
	t.Cleanup(func() { UseSQLite = was })
	UseSQLite = false
	if got := SearchBackend(); !strings.Contains(got, "rewritten whole") {
		t.Errorf("nothing says what the JSON index costs: %q", got)
	}
}

// The worst button in the product, and why it cannot exist.
//
// With SQLite off, index.db is unwritten — and it is also the only copy of
// however many months of archive. "Unwritten" and "safe to delete" are not the
// same property, which is the entire reason Stale and Superseded are two
// functions and not one.
func TestSupersededNeverNamesTheOnlyCopy(t *testing.T) {
	was := UseSQLite
	t.Cleanup(func() { UseSQLite = was })

	UseSQLite = false
	if got := Superseded(); len(got) != 0 {
		t.Fatalf("with the JSON index live, %v was offered for deletion", got)
	}
	// Named as unwritten, which is a display fact — and never as removable.
	for _, name := range Stale() {
		if name == "index.db" && contains(Superseded(), "index.db") {
			t.Error("the SQLite archive is offered for deletion while it is the live store")
		}
	}
}

// Nothing is removable until the migration has actually run. Before that,
// index.json is the input to it rather than a copy of its output.
func TestNothingIsSupersededUntilTheMigrationHasRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	was := UseSQLite
	t.Cleanup(func() { UseSQLite = was })

	UseSQLite = true
	// An empty SQLite index: IndexStats reports nothing, or fails outright
	// because there is no database at all. Both mean the same thing here.
	if got := Superseded(); len(got) != 0 {
		t.Errorf("with an empty index, %v was called superseded", got)
	}
	// And so removing does nothing rather than something.
	removed, freed, err := RemoveSuperseded()
	if err != nil || len(removed) != 0 || freed != 0 {
		t.Errorf("removed %v (%d bytes), err %v — want nothing touched", removed, freed, err)
	}
}

// A file that is not there is not an error. The operator pressed the button
// twice, or went to the box with rm first.
func TestRemovingWhatIsAlreadyGoneIsQuiet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".mu", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	removed, _, err := RemoveSuperseded()
	if err != nil {
		t.Errorf("removing nothing reported %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed %v from an empty directory", removed)
	}
}

func contains(all []string, want string) bool {
	for _, s := range all {
		if s == want {
			return true
		}
	}
	return false
}
