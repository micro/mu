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
	threads := thread.List(accountID, previewShown)
	if len(threads) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<div class="inbox-peek">`)
	for _, t := range threads {
		title := t.Subject
		if title == "" {
			title = "Untitled"
		}

		// Where it happened, when that is not here. A row saying "web" on the
		// page you are already on is noise; a row saying "Email" is the fact
		// worth showing, because it happened without you.
		where := ""
		if t.Client != thread.WebClient {
			where = `<span class="peek-where">` + html.EscapeString(thread.ClientName(t.Client)) + `</span>`
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

		b.WriteString(`<a class="peek-row" href="/inbox?session=` + url.QueryEscape(t.ID) + `">` +
			`<span class="peek-head"><span class="peek-title">` + html.EscapeString(trimTo(title, 60)) + `</span>` +
			where + `<span class="peek-when">` + html.EscapeString(app.TimeAgo(t.Updated)) + `</span></span>` +
			`<span class="peek-line">` + line + `</span></a>`)
	}
	b.WriteString(`<a class="peek-more" href="/inbox">Go to inbox &rarr;</a>`)
	b.WriteString(`</div>` + previewCSS)
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

const previewCSS = `<style>
.inbox-peek{margin:10px 0 0;border-top:1px solid #eee;padding-top:10px}
.peek-row{display:block;padding:7px 0;text-decoration:none;color:inherit;border-bottom:1px solid #f4f4f4}
.peek-row:hover .peek-title{text-decoration:underline}
.peek-head{display:flex;align-items:baseline;gap:8px}
.peek-title{font-size:14px;font-weight:500;color:#111;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.peek-where{border:1px solid #eee;border-radius:999px;padding:1px 7px;font-size:10px;color:#777;white-space:nowrap;flex:none}
.peek-when{font-size:11px;color:#bbb;margin-left:auto;white-space:nowrap;flex:none}
.peek-line{display:block;font-size:13px;color:#888;margin-top:2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.peek-who{color:#aaa}
.peek-more{display:inline-block;margin-top:9px;font-size:13px;color:#555;text-decoration:none}
.peek-more:hover{color:#000}
</style>`
