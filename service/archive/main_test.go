package archive

// A home of its own, for the whole package.
//
// These tests assert exact counts — "4 entries", "news: 2" — against the
// shared index, which was fine while the index was an in-memory map rebuilt
// per process. It is SQLite on disk now, at ~/.mu/data/index.db, and that file
// is the developer's real one: it survives between runs, and every other
// package's tests write into it too. So the counts drifted upward until they
// stopped matching, and the failure looked like an archive bug rather than a
// test reaching into somebody's data directory.
//
// TestMain rather than t.Setenv, because internal/data opens the database once
// on first use and keeps the handle. By the time a test body runs it is too
// late to say where it should live.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mu-archive-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
