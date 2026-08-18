package data

import (
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

func contains(all []string, want string) bool {
	for _, s := range all {
		if s == want {
			return true
		}
	}
	return false
}
