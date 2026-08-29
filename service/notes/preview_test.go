package notes

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/notes"
)

// req is a request to render against — the board carries a form, so it needs
// one for the CSRF token.
func req() *http.Request { return httptest.NewRequest("GET", "/home", nil) }

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
	if got := Preview(req(), ""); got != "" {
		t.Errorf("a preview for no account rendered %q", got)
	}
}

// A board with nothing on it is furniture.
func TestAnAccountWithNoNotesRendersNothing(t *testing.T) {
	// The store writes through to disk; keep it out of the live ~/.mu.
	t.Setenv("HOME", t.TempDir())
	if got := Preview(req(), "nobody-has-written-here"); got != "" {
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

	got := Preview(req(), who)
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

	got := Preview(req(), who)
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

	got := Preview(req(), who)
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

// Every row offers a way to take that note down.
//
// The board is where what your agents know becomes visible, and a thing you can
// see but not change is a display, not a control. Every note here goes into the
// system prompt of every question this account asks, so taking one down is the
// only edit on this screen that matters.
func TestEachRowCanBeTakenDown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "tidy"
	notes.Add(who, "one", "first")
	notes.Add(who, "two", "second")

	got := Preview(req(), who)
	if n := strings.Count(got, "notes-peek-take"); n != 2 {
		t.Errorf("%d rows offer to come down, want 2", n)
	}
	// The same delete the page uses, posted to the same handler.
	if !strings.Contains(got, `name="delete" value="two"`) {
		t.Error("a row does not name the note it would take down")
	}
	// It asks first, as the page does: a note is not archived anywhere.
	if !strings.Contains(got, "confirm(") {
		t.Error("taking a note down does not ask first")
	}
	// And it is a POST that carries the token, not a link somebody else's page
	// can put in an image tag.
	if !strings.Contains(got, `method="post"`) || !strings.Contains(got, `name="_csrf"`) {
		t.Errorf("the control is not a guarded POST:\n%s", got)
	}
	// Coming back to where it was taken down from.
	if !strings.Contains(got, `name="back" value="/home"`) {
		t.Error("taking a note down from Home would land on /notes")
	}
}

// Opening a note and taking it down are two controls, not one.
//
// A form nested inside the anchor would be invalid markup, and a delete button
// that is part of the link is a note you lose by trying to read it.
func TestTheRowSeparatesOpeningFromTakingDown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	notes.Add("careful", "one", "first")

	got := Preview(req(), "careful")
	open := strings.Index(got, "notes-peek-open")
	form := strings.Index(got, "<form")
	if open == -1 || form == -1 {
		t.Fatalf("row is missing a control:\n%s", got)
	}
	// The anchor closes before the form opens.
	if closed := strings.Index(got, "</a>"); closed == -1 || closed > form {
		t.Errorf("the form is inside the link:\n%s", got)
	}
}

// Where taking a note down lands, decided by the handler.
//
// "back" arrives on a form, so it is whatever the sender says it is. It is
// compared against the one value that is allowed rather than followed: a
// redirect target taken on trust is an open redirect, and this one is reachable
// by anybody who can get a person to submit a form.
func TestTakingANoteDownGoesBackOnlyToAPlaceWeNamed(t *testing.T) {
	for _, tc := range []struct{ back, want string }{
		{"/home", "/home"},
		{"", "/notes"},
		{"/notes", "/notes"},
		{"https://evil.example/steal", "/notes"},
		{"//evil.example", "/notes"},
		{"/home/../../admin", "/notes"},
		{"/home?x=1", "/notes"},
	} {
		t.Setenv("HOME", t.TempDir())
		notes.Add("poster", "gone", "text")

		form := url.Values{"_csrf": {"t"}, "delete": {"gone"}, "back": {tc.back}}
		r := httptest.NewRequest("POST", "/notes", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		handlePost(w, r, "poster")

		if got := w.Header().Get("Location"); got != tc.want {
			t.Errorf("back=%q redirected to %q, want %q", tc.back, got, tc.want)
		}
		if notes.Get("poster", "gone") != "" {
			t.Errorf("back=%q: the note was not taken down", tc.back)
		}
	}
}
