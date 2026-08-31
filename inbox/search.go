package inbox

// Looking for a conversation.
//
// thread.Search has existed the whole time — exported, complete, a case-
// insensitive scan over every message in the account's record — and the only
// door onto it was /recall, a separate page in a separate service. So the
// screen holding the conversations had no way to look through them, and finding
// one meant knowing that another page searched the same store. That is two
// facts nothing on the inbox said.
//
// This is the same read, on the screen where the question gets asked. It
// searches the record rather than the page: every channel, every conversation
// the account has, not the twenty-five rows currently drawn.

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/thread"
)

// searchHits is how many messages the scan may return. Threads collapse below
// this, so it is a ceiling on the work rather than on the answer.
const searchHits = 200

// searchBox is the field, carrying whatever was typed so a result page shows
// its own query.
//
// # It posts
//
// It was a GET, on the argument that a search should be a URL — linkable,
// reloadable, back-buttonable, which is what somebody comparing two searches
// does. That argument is right about public search and wrong here, and the
// difference is what the words are.
//
// This box searches somebody's mail. A GET writes what they typed into the URL,
// and a URL comes to rest in four places: our own request log (which records
// the path and not the query, so not there), a Referer (no-referrer on every
// page, so not there), the browser's history — which syncs to an account — and
// the access log of whatever terminates TLS in front of us. That last one is
// the one that matters, because a self-hosted install has an nginx or a Caddy in
// front and both log the full URI by default. So "search my mail for the
// solicitor's name" was being written in plaintext to /var/log on the owner's
// own machine, by software we do not control, on a product whose premise is that
// the data is theirs.
//
// The price is that a result page cannot be linked to, and for a search over
// your own mail that is closer to a feature: back goes to the unsearched inbox,
// which is where somebody who has finished looking wants to be.
//
// See AGENTS.md, "What may travel in a URL".
func searchBox(box, q, csrf string) string {
	// The box is preserved across a search, because a search inside a mailbox
	// is a narrower question than a search across all of them and the switcher
	// above has just been used to ask it.
	action := boxPath(box)
	if action == "" {
		action = "/inbox"
	}
	return `<form class="ib-search" method="POST" action="` + html.EscapeString(action) + `">` +
		app.CSRFField(csrf) +
		`<input type="search" name="q" placeholder="Search your conversations" ` +
		`value="` + html.EscapeString(q) + `" autocomplete="off" class="ib-search-in">` +
		`<button type="submit" class="btn btn-quiet">Search</button>` +
		clearSearch(action, q) +
		`</form>`
}

// clearSearch is the way back to the mailbox, offered only when there is
// something to go back from.
func clearSearch(action, q string) string {
	if q == "" {
		return ""
	}
	return `<a class="ib-search-clear" href="` + html.EscapeString(action) + `">Clear</a>`
}

// found renders the results: the conversations that matched, and the line in
// each that did.
//
// Grouped by conversation rather than listed as messages. A search that returns
// nine lines from one thread has buried the other four threads that matched
// once each, and the thing being looked for is almost always the conversation —
// "where did we talk about the invoice" — rather than the sentence.
func found(b *strings.Builder, r *http.Request, accountID, box, q string) {
	hits := thread.Search(accountID, q, "", searchHits)

	// First hit per thread wins, and the scan already sorted newest first, so
	// the line shown is the most recent time it was said.
	seen := map[string]bool{}
	type result struct {
		t    thread.Thread
		line string
	}
	var out []result
	for _, h := range hits {
		if seen[h.Thread] {
			continue
		}
		seen[h.Thread] = true
		t := thread.Get(accountID, h.Thread)
		if t == nil {
			continue
		}
		// The mailbox narrows the search, because the switcher above is how the
		// question was asked. Applied here rather than by passing a client to
		// thread.Search: a box is an agent, not a channel.
		if box != "" && !strings.EqualFold(boxOfThread(accountID, *t), box) {
			continue
		}
		// Only quote the line when the line is what matched. A hit on the
		// subject or on the sender points at a conversation whose messages need
		// not contain the query at all, and showing one of them as though it
		// were the match reads as a broken search — which is what a DMARC
		// report would do, its body being "(no message — attached: …)".
		line := ""
		if h.Where == "" || h.Where == thread.InText {
			line = h.Text
		}
		out = append(out, result{t: *t, line: line})
	}

	if len(out) == 0 {
		b.WriteString(`<p class="ib-empty">Nothing matches <code>` +
			html.EscapeString(trimTo(q, 60)) + `</code>`)
		if box != "" {
			b.WriteString(` in <code>` + html.EscapeString(box) + `</code>`)
		}
		b.WriteString(`. This looks at what was said, what the conversation is ` +
			`called and who is on it, across every channel.</p>`)
		return
	}

	b.WriteString(`<p class="ib-found">` + count(len(out)) + ` for <code>` +
		html.EscapeString(trimTo(q, 60)) + `</code>.</p>`)
	for _, res := range out {
		b.WriteString(rowWith(r, accountID, res.t, res.line))
	}
	// No pager. The scan is capped at searchHits and collapses to fewer
	// conversations than that, so there is no second page to offer — and an
	// offer of one that did nothing is worse than its absence.
}

// count says how many, in words, because "1 conversations" is the sort of thing
// that makes a page look unfinished.
func count(n int) string {
	if n == 1 {
		return "One conversation"
	}
	return itoa(n) + " conversations"
}

func itoa(n int) string {
	if n <= 0 {
		return "No"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}
