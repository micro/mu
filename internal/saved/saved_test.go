package saved

import (
	"errors"
	"fmt"
	"mu/internal/data"
	"strings"
	"sync"
	"testing"
)

func TestOwnershipPersistenceAndSearch(t *testing.T) {
	owner := t.Name()
	defer Clear(owner)
	item, err := Add(owner, Item{URL: "https://example.com/story#section", Title: "An article", Note: "private needle"})
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []string{"", "other"} {
		if _, err := Get(other, item.ID); err == nil {
			t.Fatal("another caller read the item")
		}
		if err := Annotate(other, item.ID, "overwrite"); err == nil {
			t.Fatal("another caller edited it")
		}
		if err := Remove(other, item.ID); err == nil {
			t.Fatal("another caller removed it")
		}
	}
	duplicate, err := Add(owner, Item{URL: "https://example.com/story", Title: "new title"})
	if err != nil || duplicate.ID != item.ID || duplicate.Note != "private needle" {
		t.Fatalf("re-save lost identity or note: %+v %v", duplicate, err)
	}
	for i := 0; i < 25; i++ {
		if _, err := Add(owner, Item{URL: fmt.Sprintf("https://example.com/%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	matches, total, err := List(owner, "needle", "", 0, 20)
	if err != nil || total != 1 || matches[0].ID != item.ID {
		t.Fatalf("search lost an older item: %v %d", err, total)
	}
	page, total, err := List(owner, "", "", 20, 20)
	if err != nil || total != 26 || len(page) != 6 {
		t.Fatalf("pagination: %v %d %d", err, total, len(page))
	}
	if err := Annotate(owner, item.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := Remove(owner, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(owner, item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted item survived")
	}
	if err := Clear(owner); err != nil {
		t.Fatal(err)
	}
	_, total, err = List(owner, "", "", 0, 20)
	if err != nil || total != 0 {
		t.Fatal("account deletion left data")
	}
}
func TestConcurrentSaveIsIdempotent(t *testing.T) {
	owner := t.Name()
	defer Clear(owner)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Add(owner, Item{URL: "https://example.com/one"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	_, total, err := List(owner, "", "", 0, 20)
	if err != nil || total != 1 {
		t.Fatalf("concurrent duplicates: %d %v", total, err)
	}
}
func TestUnsafeLinksAndStorageFailure(t *testing.T) {
	for _, u := range []string{"javascript:alert(1)", "//evil.com", "file:///etc/passwd", "https://user:secret@example.com", "https://"} {
		if _, err := Add(t.Name(), Item{URL: u}); err == nil {
			t.Fatalf("accepted %q", u)
		}
	}
	owner := t.Name()
	k, _ := key(owner)
	if err := data.SaveFile(k, "corrupt"); err != nil {
		t.Fatal(err)
	}
	defer data.DeleteFile(k)
	if _, err := Add(owner, Item{URL: "https://example.com"}); err == nil {
		t.Fatal("overwrote unreadable data")
	}
}
func TestPublicSourceAndDurableMetadata(t *testing.T) {
	owner := t.Name()
	defer Clear(owner)
	for _, entry := range []struct{ id, kind, who string }{{"saved-test-public", data.KindNews, ""}, {"saved-test-private", data.KindNews, "elsewhere"}, {"saved-test-note", data.KindNote, ""}} {
		if err := data.IndexSQLite(entry.id, entry.kind, "Source title", "Source body", entry.who, map[string]any{"url": "https://example.com/source"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, ref := range []string{"saved-test-private", "saved-test-note"} {
		if _, err := Source(ref); err == nil {
			t.Fatal("accepted private or unsupported source")
		}
	}
	item, err := Add(owner, Item{Ref: "saved-test-public"})
	if err != nil {
		t.Fatal(err)
	}
	data.Unindex(item.Ref)
	got, err := Get(owner, item.ID)
	if err != nil || got.Title != "Source title" || !strings.Contains(Context(got), "Source body") {
		t.Fatalf("lost saved metadata: %+v %v", got, err)
	}
}
