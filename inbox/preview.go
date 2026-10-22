package inbox

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

// Preview shows incoming conversations on Home. Personal web and CLI chats
// belong in the adjacent Chats card, including when their replies are unread.
func Preview(accountID string) string {
	if accountID == "" {
		return ""
	}
	var all []thread.Thread
	for _, t := range thread.List(accountID, 0) {
		if thread.Arrived(t) {
			all = append(all, t)
			if len(all) == previewShown {
				break
			}
		}
	}
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
	b.WriteString(`</div>`)
	return b.String()
}

// trimTo shortens text to fit a line, on a word where it can.
//
// Whitespace is collapsed first: a message that arrived by mail carries the
// newlines it was typed with, and a snippet rendered with them in it is three
// short lines where the layout has room for one.
//
// Markdown comes off first. A preview is escaped and drawn as text, so what
// the agent writes appeared on Home literally:
//
//	@micro ?
//	Agent **@micro** is an agent on this instance. - **Na
//
// — the asterisks, the bullet, and the cut landing mid-word inside one. The
// thread renders the markdown properly; the row has one line and no renderer,
// so it gets the words.

// plain is markdown with the marks taken off, for somewhere there is no
// renderer.
//
// Not a parser. It undoes the handful of things a model actually writes —
// emphasis, code ticks, headings, bullets, numbered items, links — and leaves
// anything it does not recognise alone, because a preview that mangles an
// unusual message is worse than one that shows a stray character. A `#` in the
// middle of a sentence is a `#`; only one at the start of a line is a heading.

// Waiting counts unread conversations in the same local selection as Inbox.
// Unread is read state, not a claim that a reply or action is needed.
func Waiting(accountID string) (unread int, newest string) {
	for _, t := range inboxThreads(accountID, "/inbox") {
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
