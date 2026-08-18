package blog

import (
	"testing"

	"mu/internal/app"
)

// Hiding a post takes it out of the list, which is the whole promise of the
// control.
//
// It did not. The blog page was one HTML blob rebuilt when posts change and
// handed to everybody, so by the time a request arrived there was no post left
// to skip — only a string. You could hide something you had written yourself
// and still be looking at it, with a row in /user saying it was hidden.
func TestHiddenPostsLeaveTheList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	items := []listItem{
		{ID: "1", AuthorID: "ann", HTML: "<p>one</p>"},
		{ID: "2", AuthorID: "bob", HTML: "<p>two</p>"},
	}

	if got := visibleTo("", items); len(got) != 2 {
		t.Error("a signed-out viewer has hidden nothing and should get the whole list")
	}
	if got := visibleTo("viewer", items); len(got) != 2 {
		t.Error("a viewer who has hidden nothing should get the whole list")
	}

	app.DismissItem("viewer", "post", "1")
	got := visibleTo("viewer", items)
	if len(got) != 1 {
		t.Fatalf("the hidden post is still in the list: %d items", len(got))
	}
	if want := "<p>two</p>"; joinItems(got) != want {
		t.Errorf("visibleTo = %q, want %q", joinItems(got), want)
	}

	app.BlockUser("viewer", "bob")
	if got := visibleTo("viewer", items); len(got) != 0 {
		t.Errorf("blocking the other author should empty the list, got %d items", len(got))
	}
}
