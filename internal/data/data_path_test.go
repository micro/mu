package data

import "testing"

func TestDataPathConfines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Keys that would escape the store must be rejected.
	escape := []string{
		"../escape.json",
		"a/../../escape.json",
		"apps/../../../etc/passwd",
		"..",
		"a/../..",
	}
	for _, k := range escape {
		if _, err := dataPath(k); err == nil {
			t.Errorf("dataPath(%q) allowed, want rejection", k)
		}
	}

	// Normal keys (including nested app/collection paths) must be allowed.
	ok := []string{
		"apps.json",
		"apps/notes/db/notes.json",
		"apps/notes/alice.json",
		"discord_links.json",
	}
	for _, k := range ok {
		if _, err := dataPath(k); err != nil {
			t.Errorf("dataPath(%q) rejected: %v", k, err)
		}
	}
}
