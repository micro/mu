package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The operator can ask the running instance what is on disk.
//
// Nothing could. Every store here is a whole-file blob rewritten in one go, so
// a file that has quietly become the largest thing in the directory is a page
// or a background loop paying for its whole size on every write — and the only
// way to find that out was to ssh in and run ls.
func TestTheStoresListWhatIsOnDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".mu", "data")
	if err := os.MkdirAll(filepath.Join(dir, "news", "metadata"), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, n int) {
		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, n), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("small.json", 10)
	write("big.json", 5000)
	write("news/metadata/a.json", 100)
	write("news/metadata/b.json", 200)

	stores := Stores()
	if len(stores) != 3 {
		t.Fatalf("listed %d stores, want 3 (two files and one directory): %+v", len(stores), stores)
	}

	// Largest first, which is the whole point of the list.
	if stores[0].Name != "big.json" || stores[0].Size != 5000 {
		t.Errorf("the largest store is not first: %+v", stores[0])
	}

	// A directory is one row with its contents summed.
	var metadata *Store
	for i := range stores {
		if strings.HasPrefix(stores[i].Name, "news") {
			metadata = &stores[i]
		}
	}
	if metadata == nil {
		t.Fatal("the news directory is missing from the list")
	}
	if metadata.Size != 300 || metadata.Files != 2 {
		t.Errorf("news/ = %+v, want 300 bytes over 2 files", *metadata)
	}
	if !strings.HasSuffix(metadata.Name, "/") {
		t.Errorf("a directory is not marked as one: %q", metadata.Name)
	}
}

// A data directory that does not exist yet is not an error — it is a fresh
// install, and the page it feeds still has to render.
func TestNoDataDirectoryListsNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got := Stores(); got != nil {
		t.Errorf("a fresh install lists %+v", got)
	}
}
