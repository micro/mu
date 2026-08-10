package notes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/notes"
)

// The page is a notes UI: a list you click into and an editor with a body.
//
// The first version was the /account memory card moved across — a row per note
// and a one-line form reading "Remember that my … is …", two same-width boxes
// and no way to write more than a phrase. That is a memory widget wearing the
// word notes.
func TestTheListLinksIntoAnEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notes.Add("someone", "shopping", "milk, bread, and something for Sunday")

	body := list(notes.All("someone"))

	if !strings.Contains(body, `href="/notes?note=shopping"`) {
		t.Error("a note in the list does not open")
	}
	if !strings.Contains(body, "New note") {
		t.Error("no way to write a new note")
	}
	if strings.Contains(body, "Remember that my") {
		t.Error("the fill-in-the-blank memory form is back")
	}
}

func TestTheEditorHasABody(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/notes?note=shopping", nil)

	body := editor(r, "shopping", "milk, bread")
	if !strings.Contains(body, "<textarea") {
		t.Error("a note is written in a textarea, not an input")
	}
	if !strings.Contains(body, "milk, bread") {
		t.Error("the editor does not show what the note says")
	}
	if !strings.Contains(body, "Delete") {
		t.Error("no way to delete the note you are looking at")
	}

	// A new note needs a title field; an existing one is addressed by its
	// title, so that field is fixed and posted as it stands.
	if fresh := editor(r, "", ""); !strings.Contains(fresh, `name="title" class="note-title-in"`) ||
		strings.Contains(fresh, `readonly aria-label`) {
		t.Error("a new note should ask for a title")
	}
}

// Titles are not URL-safe by nature — "project brief" has a space in it — and a
// link that drops half the title opens the wrong note or none.
func TestTitlesWithSpacesStillOpen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notes.Add("someone", "project brief", "the whole thing")

	if got := list(notes.All("someone")); !strings.Contains(got, "note=project+brief") {
		t.Errorf("a two-word title is not linked safely: %s", got)
	}
}
