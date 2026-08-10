package blog

import (
	"testing"

	"mu/internal/app"
)

// Hiding a post takes it out of the list, which is the whole promise of the
// control.
//
// It did not. The blog page is one HTML blob rebuilt when posts change and
// handed to everybody, so by the time a request arrived there was no post left
// to skip — only a string. You could hide something you had written yourself
// and still be looking at it, with a row in /user saying it was hidden.
func TestHiddenPostsLeaveTheList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	items := []listItem{
		{ID: "1", AuthorID: "ann", HTML: "<p>one</p>"},
		{ID: "2", AuthorID: "bob", HTML: "<p>two</p>"},
	}
	all := "<p>one</p>\n<p>two</p>"

	if got := visibleTo("", items, all); got != all {
		t.Error("a signed-out viewer has hidden nothing and should get the shared list")
	}
	if got := visibleTo("viewer", items, all); got != all {
		t.Error("a viewer who has hidden nothing should get the shared list")
	}

	app.DismissItem("viewer", "post", "1")
	got := visibleTo("viewer", items, all)
	if got == all {
		t.Fatal("the hidden post is still in the list")
	}
	if want := "<p>two</p>"; got != want {
		t.Errorf("visibleTo = %q, want %q", got, want)
	}

	app.BlockUser("viewer", "bob")
	if got := visibleTo("viewer", items, all); got != "<p>Nothing to show — you have hidden everything here.</p>" {
		t.Errorf("blocking the other author should empty the list, got %q", got)
	}
}
