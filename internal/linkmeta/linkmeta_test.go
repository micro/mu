package linkmeta

import (
	"os"
	"path/filepath"
	"testing"
)

// TestThePathIsStillNewsMetadata guards a live cache.
//
// Every running instance already has news/metadata/ full of link previews. The
// package moved; the files did not, and renaming the directory to match the
// package would read as tidying while throwing away every cached preview on
// every instance — silently, because a cache miss looks exactly like a link
// nobody has seen before.
func TestThePathIsStillNewsMetadata(t *testing.T) {
	got := Path("https://example.com/a")
	if dir := filepath.Dir(got); dir != filepath.Join("news", "metadata") {
		t.Fatalf("metadata path is %q — the existing cache lives in news/metadata", dir)
	}
	if Path("https://example.com/a") != Path("https://example.com/a") {
		t.Error("the same URL hashed to two different files")
	}
	if Path("https://example.com/a") == Path("https://example.com/b") {
		t.Error("two URLs share one file")
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".mu", "data"), 0o755); err != nil {
		t.Fatal(err)
	}

	const uri = "https://example.com/story"
	if _, ok := Lookup(uri); ok {
		t.Fatal("found metadata for a link nobody has fetched")
	}
	if err := Save(uri, &Metadata{Title: "A story", Site: "example.com"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	md, ok := Lookup(uri)
	if !ok || md.Title != "A story" || md.Site != "example.com" {
		t.Errorf("Lookup = %+v, %v", md, ok)
	}
}
