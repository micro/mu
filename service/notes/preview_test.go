package notes

import (
	"strings"
	"testing"

	"mu/internal/notes"
)

// Nobody in particular gets nobody's notes.
//
// This is the whole reason the board is not a card. Home's card grid renders
// once as service.Anyone and serves that one string to every viewer, so a notes
// card would have put whoever refreshed the cache on everybody's home screen.
// The signature that makes that mistake possible is an empty account, so the
// empty account must render nothing.
func TestAPreviewForNobodyIsEmpty(t *testing.T) {
	// The store writes through to disk; keep it out of the live ~/.mu.
	t.Setenv("HOME", t.TempDir())
	notes.Add("someone", "location", "London")
	if got := Preview(""); got != "" {
		t.Errorf("a preview for no account rendered %q", got)
	}
}

// A board with nothing on it is furniture.
func TestAnAccountWithNoNotesRendersNothing(t *testing.T) {
	// The store writes through to disk; keep it out of the live ~/.mu.
	t.Setenv("HOME", t.TempDir())
	if got := Preview("nobody-has-written-here"); got != "" {
		t.Errorf("an empty board rendered %q", got)
	}
}

// The newest change first, bounded, and each row opens that note.
func TestThePreviewShowsTheLatestNotesAndLinksToEach(t *testing.T) {
	// The store writes through to disk; keep it out of the live ~/.mu.
	t.Setenv("HOME", t.TempDir())
	const who = "boardreader"
	for _, title := range []string{"one", "two", "three", "four", "five", "six"} {
		notes.Add(who, title, "text of "+title)
	}

	got := Preview(who)
	if got == "" {
		t.Fatal("notes were written and the board is empty")
	}

	// Bounded: a board is not a feed.
	if n := strings.Count(got, "notes-peek-row"); n != previewShown {
		t.Errorf("board shows %d notes, want %d", n, previewShown)
	}
	// Newest first, so the last one written is up there and the first is not.
	if !strings.Contains(got, "?note=six") {
		t.Error("the note written last is not on the board")
	}
	if strings.Contains(got, "?note=one") {
		t.Error("the oldest note is on the board, so the order is wrong")
	}
	// And each row goes to that note, not to the list.
	if !strings.Contains(got, `href="/notes?note=`) {
		t.Error("a row does not open the note it shows")
	}
	if !strings.Contains(got, "/notes") {
		t.Error("no way through to the page")
	}
}

// A title with a space or an ampersand still opens.
//
// Notes are addressed by title and a person names one whatever they like, so
// the link is a query value and has to be escaped as one. Writing it raw made
// "shopping list" a link to /notes?note=shopping.
func TestATitleThatNeedsEscapingStillLinks(t *testing.T) {
	// The store writes through to disk; keep it out of the live ~/.mu.
	t.Setenv("HOME", t.TempDir())
	const who = "awkward"
	notes.Add(who, "shopping list & bills", "milk")

	got := Preview(who)
	if !strings.Contains(got, "?note=shopping+list+%26+bills") {
		t.Errorf("the title is not escaped into the link: %s", got)
	}
	if strings.Contains(got, "& bills\"") {
		t.Error("the title reached the href unescaped")
	}
}

// The text on a row is one line.
func TestALongNoteIsTrimmedToALine(t *testing.T) {
	// The store writes through to disk; keep it out of the live ~/.mu.
	t.Setenv("HOME", t.TempDir())
	const who = "verbose"
	notes.Add(who, "brief", strings.Repeat("a very long sentence ", 40))

	got := Preview(who)
	if !strings.Contains(got, "…") {
		t.Error("a long note was not trimmed")
	}
	if len(got) > 4000 {
		t.Errorf("one note rendered %d bytes, so it is not trimmed to a line", len(got))
	}
}

// Notes has no card, and must not grow one.
//
// A card would be the natural-looking way to put notes on Home, and it is the
// one that leaks: home.RefreshCards renders every card once as service.Anyone
// and hands that one string to every viewer, so whoever's refresh happened to
// populate the cache would be showing their notes to the instance. The board on
// Home is rendered per request instead — see Preview.
func TestNotesIsNotACard(t *testing.T) {
	if Spec.Card.Set() {
		t.Error("notes declares a Card; the home card cache is shared by every " +
			"viewer, so one account's notes would render on everybody's screen")
	}
}
