package notes

// Notes, on the screen you arrive at.
//
// This service is already the thing a fridge door is: you write on it, your
// agents write on it, and everything on it is read back into every question you
// ask. What it never had was a wall. It rendered at /notes and nowhere else, so
// the one surface both parties write to was the one surface neither party
// walked past — and a note an agent left after working while you were out was
// indistinguishable from no note at all.
//
// It is not a card. Home's card grid is one cache of strings shared by every
// viewer (see home.RefreshCards, which renders every card as service.Anyone),
// and notes are one person's. A card would put your notes on everybody's home
// screen. So this goes on the per-request half of that page, beside the inbox
// and the agent roster — which is where it belongs anyway: those two are the
// other "what is yours" blocks, and this is the third.

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/notes"
)

// previewShown is how many notes Home carries.
//
// A board is bounded or it is a feed. Four is enough to show that something is
// up there without turning the screen you arrive at into the notes page, which
// is one link below.
const previewShown = 4

// previewSnippet bounds the line of text under a title.
const previewSnippet = 80

// Preview is the most recent notes, for Home. Empty when there are none: a
// board with nothing pinned to it is furniture, and the page says what makes a
// note appear better than an empty box on Home could.
func Preview(r *http.Request, accountID string) string {
	if accountID == "" {
		return ""
	}
	entries := notes.All(accountID)
	if len(entries) == 0 {
		return ""
	}

	// Newest change first, the same order the page uses: the note last touched
	// is the one most likely to be the reason you are looking. The store keeps
	// them in the order they were first written.
	ordered := make([]*notes.Entry, len(entries))
	copy(ordered, entries)
	for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
		ordered[i], ordered[j] = ordered[j], ordered[i]
	}
	if len(ordered) > previewShown {
		ordered = ordered[:previewShown]
	}

	csrf := html.EscapeString(auth.CSRFToken(r))

	var b strings.Builder
	b.WriteString(`<div class="notes-peek">`)
	for _, e := range ordered {
		b.WriteString(`<div class="notes-peek-row">`)

		// ?note=<title> is what the page reads to open the editor — a note is
		// addressed by its title here as it is everywhere else in this service.
		b.WriteString(`<a class="notes-peek-open" href="/notes?note=` +
			url.QueryEscape(e.Title) + `">`)
		// Title and time on the first line, the note itself under them. The
		// time was after the text, which put it in the middle of the row on a
		// column this narrow and left the title with nothing on its right.
		b.WriteString(`<span class="notes-peek-title">` +
			html.EscapeString(trimTo(e.Title, 30)) + `</span>`)
		b.WriteString(`<span class="notes-peek-when">` +
			html.EscapeString(app.TimeAgo(e.UpdatedAt)) + `</span>`)
		b.WriteString(`<span class="notes-peek-text">` +
			html.EscapeString(trimTo(e.Text, previewSnippet)) + `</span>`)
		b.WriteString(`</a>`)

		// Taking a note down, from where it is pinned.
		//
		// The one thing you can do to the board without leaving the screen you
		// arrived at, and the reason it is worth having is not tidiness. Every
		// note here is read back into the system prompt of every question this
		// account asks, so what the agent knows has been invisible: fifty notes
		// shaping every answer, on a page you had to go and find. On the wall
		// it is legible, and this makes it editable — one click, and the agent
		// stops being told.
		//
		// Which is why it asks first, and why it says Remove. This is the same
		// delete the page offers, with the same confirm on it, because it is
		// the same act: a note is not archived anywhere and removing it is the
		// whole of losing it. Not a cross, which is the glyph for close and
		// hide — reversible things — and not Dismiss, which says the same. The
		// word has to mean the thing that happens.
		//
		// Under the note rather than beside it. Beside it, the control competes
		// with the time for the right-hand end of a 360px column, and the two
		// most different things on the row — when it changed, and the one way
		// to lose it — end up in the same place.
		b.WriteString(`<form method="post" action="/notes" class="notes-peek-take" ` +
			`onsubmit="return confirm('Take this note down? Your agents stop being told it.')">` +
			`<input type="hidden" name="_csrf" value="` + csrf + `">` +
			`<input type="hidden" name="back" value="/home">` +
			`<button type="submit" name="delete" value="` + html.EscapeString(e.Title) +
			`" aria-label="Remove the note called ` +
			html.EscapeString(e.Title) + `">Remove</button>` +
			`</form>`)

		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(app.Link("Go to notes", "/notes"))
	return b.String()
}

// trimTo shortens text to fit a line, on a word where it can.
//
// Whitespace is collapsed first: a note is a body somebody typed newlines into,
// and a snippet rendered with them in it is four short lines where the row has
// room for one.
func trimTo(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}
