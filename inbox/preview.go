package inbox

// The inbox, on the screen you arrive at.
//
// Home said "Or write to it at <address> — Your inbox", and the link was the
// whole of it: a word pointing at a page. So the one thing the product claims —
// that the agent is reachable and answers whether or not you have this page
// open — was only ever visible to somebody who clicked through to check. If mail
// arrived overnight and was answered, Home looked exactly as it had the night
// before.
//
// There is an older comment in home/home.go saying an inbox preview is "three
// subject lines beside a Mail page one click away", and that was right about
// what it was written about: a preview of *mail*, next to a Mail page. This is
// the record — every conversation this account has had with an agent, on
// whichever client it arrived — and it is what Home is now for.

import (
	"html"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/thread"
)

// previewShown is how many conversations Home carries.
//
// Enough to show that something happened while you were away, not so many that
// Home becomes the inbox. The page for reading them is one link below.
const previewShown = 3

// previewSnippet bounds the line of text under a subject.
const previewSnippet = 90

// Preview is the most recent conversations, for Home. Empty when there are
// none — an empty list is worse than the address line on its own, which at least
// says what to do about it.
func Preview(accountID string) string {
	if accountID == "" {
		return ""
	}
	// What arrived, the same as the page it previews. It listed every
	// conversation, so Home showed the chat you had here thirty seconds ago
	// under a heading that says these are things that turned up while you were
	// away — see thread.Arrived.
	all := arrivals(accountID)
	if len(all) == 0 {
		return ""
	}
	threads := all
	if len(threads) > previewShown {
		threads = threads[:previewShown]
	}

	var b strings.Builder
	b.WriteString(`<div class="inbox-peek">`)
	for _, t := range threads {
		title := t.Subject
		if title == "" {
			title = "Untitled"
		}

		// Which channel carried it, as quiet text rather than a pill.
		//
		// Renamed off peek-where deliberately: that name is on the retired-pill
		// list in test/style_test.go, and reusing it for a shape that is no
		// longer a pill would leave the next reader looking for a border.
		//
		// Every row here arrived from somewhere, so the label is worth showing —
		// and shown as a pill it was the loudest thing in the block, repeated
		// three times, usually saying "Mail". It is at the end of the line the
		// eye reads last, which is where a qualifier belongs.
		where := `<span class="peek-via">` + html.EscapeString(app.ClientName(t.Client)) + `</span>`

		// And whether it is waiting for you, which is most of what a preview on
		// Home is for — three rows you have already dealt with say nothing.
		cls := "peek-row"
		if thread.Unread(t) {
			cls += " unseen"
		}

		// The last thing said, so the row is worth reading rather than just
		// worth counting. Who said it matters as much as what: "you asked" and
		// "it answered" are different states of a conversation.
		line := ""
		if msgs := thread.Messages(accountID, t.ID, 1); len(msgs) > 0 {
			m := msgs[0]
			who := "You"
			if m.Role == thread.RoleAgent {
				who = "Agent"
			} else if m.From != "" {
				who = m.From
			}
			line = `<span class="peek-who">` + html.EscapeString(who) + `</span> ` +
				html.EscapeString(trimTo(m.Text, previewSnippet))
		}

		// ?id=, which is what the inbox reads. It said ?session= and the handler
		// has never looked at that, so every row on Home opened the list.
		b.WriteString(`<a class="` + cls + `" href="/inbox?id=` + url.QueryEscape(t.ID) + `">` +
			`<span class="peek-head"><span class="peek-title">` + html.EscapeString(trimTo(title, 60)) +
			`</span><span class="peek-when">` + html.EscapeString(app.TimeAgo(t.Updated)) + `</span></span>` +
			`<span class="peek-line">` + line + where + `</span></a>`)
	}
	b.WriteString(`<a class="peek-more" href="/inbox">Go to inbox &rarr;</a>`)
	b.WriteString(`</div>`)
	return b.String()
}

// trimTo shortens text to fit a line, on a word where it can.
//
// Whitespace is collapsed first: a message that arrived by mail carries the
// newlines it was typed with, and a snippet rendered with them in it is three
// short lines where the layout has room for one.
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

// Waiting is what has arrived and not been read: how many, and who the newest
// is from.
//
// Exported because Home says it in a sentence and this package is what knows
// what "arrived" means — see arrivals. A second definition on the front page
// would drift from this one, and the two would disagree about a number sitting
// six inches apart on the same screen.
func Waiting(accountID string) (unread int, newest string) {
	for _, t := range arrivals(accountID) {
		if !thread.Unread(t) {
			continue
		}
		if unread == 0 {
			// arrivals is newest first, so the first unread one is the newest.
			newest, _ = party(accountID, t)
		}
		unread++
	}
	return unread, newest
}
