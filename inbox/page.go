package inbox

// The inbox: what arrived.
//
// /inbox was the chat — a conversation log wearing the word, so an ordinary
// email from a person about something never appeared on it at all. The chat is
// /agent/<name> now, and this is the mailbox: messages, newest first, the ones
// nobody has opened marked, and the agent's own address at the top because that
// is the thing this inbox has that the others do not.
//
// It reads service/mail and owns nothing. There is one message store, it is the
// MTA's, and a second one is the shape of the bug that has cost this repository
// the most — see docs/DIRECTION.md §8.

import (
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/mail"
)

// shown is how many messages one page of the inbox is.
const shown = 25

// held is how far back the list reaches. The store answers with a limit rather
// than a window, so this is the ceiling on what can be paged through, and it is
// generous rather than unbounded: a mailbox is not a search index, and going
// further back is what /recall is.
const held = 500

// Handler serves /inbox and /inbox/<box>.
func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	if id := r.URL.Query().Get("id"); id != "" {
		message(w, r, acc.ID, id)
		return
	}
	list(w, r, acc.ID, boxOf(r))
}

// boxOf is which mailbox the path asks for, empty for all of them.
func boxOf(r *http.Request) string {
	box := strings.Trim(strings.TrimPrefix(r.URL.Path, "/inbox"), "/")
	if strings.Contains(box, "/") {
		return ""
	}
	return mail.CleanTag(box)
}

// list is the inbox proper.
func list(w http.ResponseWriter, r *http.Request, accountID, box string) {
	all := mail.ListMessages(accountID, held)

	msgs := all
	if box != "" {
		msgs = nil
		for _, m := range all {
			if strings.EqualFold(m.Tag, box) {
				msgs = append(msgs, m)
			}
		}
	}

	var b strings.Builder
	b.WriteString(`<div class="ib">`)
	b.WriteString(addressBar(accountID))
	b.WriteString(boxes(all, box))

	if len(msgs) == 0 {
		// An empty inbox says how to fill it, and the answer is an address.
		// "No messages" is a true sentence that leaves somebody looking at a
		// blank page with nothing to do about it. An empty *box* is a narrower
		// fact and gets the narrower sentence — the address is already above it.
		if box != "" {
			b.WriteString(`<p class="ib-empty">Nothing addressed to <code>` +
				html.EscapeString(box) + `</code> yet.</p>`)
		} else {
			b.WriteString(`<p class="ib-empty">Nothing here yet. Write to the address above from ` +
				`anywhere — your own mail, your phone — and it arrives here. The agent reads what ` +
				`turns up and answers in the thread.</p>`)
		}
		b.WriteString(`</div>` + inboxCSS)
		app.Respond(w, r, app.Response{Title: "Inbox", Description: "What arrived", HTML: b.String()})
		return
	}

	pager := app.Paginate(r, len(msgs), shown)
	for _, m := range msgs[pager.From:pager.To] {
		b.WriteString(row(m))
	}
	b.WriteString(pager.Nav(boxPath(box)))
	b.WriteString(`</div>` + inboxCSS)

	app.Respond(w, r, app.Response{Title: "Inbox", Description: "What arrived", HTML: b.String()})
}

// row is one message in the list: who, what, when, and whether it has been
// opened. Unread is a weight rather than a badge — a column of dots beside
// every row is a thing to scan past, and bold is the thing you already read
// mailboxes by.
func row(m *mail.Message) string {
	cls := "ib-row"
	if !m.Read {
		cls += " ib-new"
	}
	subject := strings.TrimSpace(m.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	// The tag is how an agent's own mail is told apart — asim+research@ tags a
	// message "research" — so it is worth a chip where there is one.
	tag := ""
	if m.Tag != "" {
		tag = `<span class="ib-tag">` + html.EscapeString(m.Tag) + `</span>`
	}
	return `<a class="` + cls + `" href="/inbox?id=` + html.EscapeString(m.ID) + `">` +
		`<span class="ib-who">` + html.EscapeString(sender(m)) + `</span>` +
		`<span class="ib-subject">` + html.EscapeString(subject) + tag + `</span>` +
		`<span class="ib-when">` + html.EscapeString(app.TimeAgo(m.CreatedAt)) + `</span></a>`
}

// message is one message, read.
func message(w http.ResponseWriter, r *http.Request, accountID, id string) {
	var found *mail.Message
	for _, m := range mail.ListMessages(accountID, held) {
		if m.ID == id {
			found = m
			break
		}
	}
	if found == nil {
		// Scoped to the reader by construction: the only messages considered are
		// the ones the store returns for this account, so somebody else's id is
		// not "forbidden", it is not a thing that exists here.
		app.NotFound(w, r, "no message here with that id")
		return
	}
	// Opening it is what makes it read. Doing this on render rather than behind
	// a button is what every mail client does and what the unread count means.
	_ = mail.MarkAsRead(id, accountID)

	subject := strings.TrimSpace(found.Subject)
	if subject == "" {
		subject = "(no subject)"
	}

	var b strings.Builder
	b.WriteString(`<div class="ib">`)
	b.WriteString(`<p class="ib-back">` + app.TextLink("← Inbox", "/inbox") + `</p>`)
	b.WriteString(`<h2 class="ib-title">` + html.EscapeString(subject) + `</h2>`)
	b.WriteString(`<div class="ib-meta">` + html.EscapeString(sender(found)) +
		` · ` + html.EscapeString(app.TimeAgo(found.CreatedAt)) + `</div>`)
	// Escaped and shown as it was written. A message is text somebody else
	// composed, and the one thing it must never be is markup this page runs.
	b.WriteString(`<div class="ib-body">` + html.EscapeString(bodyText(found)) + `</div>`)
	b.WriteString(`</div>` + inboxCSS)

	app.Respond(w, r, app.Response{Title: subject, Description: "A message", HTML: b.String()})
}

// sender is who a message is from, by the address where there is one.
func sender(m *mail.Message) string {
	if s := strings.TrimSpace(m.From); s != "" {
		return s
	}
	return "Unknown"
}

// bodyText is the message as words.
func bodyText(m *mail.Message) string {
	if s := strings.TrimSpace(m.Body); s != "" {
		return s
	}
	return "(no body)"
}

// addressBar is what makes this an agentic inbox rather than a mailbox: the
// address the agent answers at, on the page where its mail lands.
func addressBar(accountID string) string {
	addr := mail.SharedAgentAddress()
	if addr == "" {
		return ""
	}
	return `<div class="ib-addr"><code>` + html.EscapeString(addr) + `</code>` +
		`<span class="ib-addr-note">Write here from anywhere and the agent answers in the thread. ` +
		app.TextLink("Your agents", "/agents") + `</span></div>`
}

const inboxCSS = `<style>
.ib{max-width:760px}
.ib-addr{display:flex;flex-wrap:wrap;align-items:baseline;gap:10px;margin:0 0 18px;padding-bottom:14px;border-bottom:1px solid #eee}
.ib-addr code{font-size:13px;background:#f5f5f5;border-radius:4px;padding:3px 8px}
.ib-addr-note{font-size:12px;color:#999}
.ib-empty{font-size:14px;color:#888;line-height:1.6}
.ib-row{display:flex;align-items:baseline;gap:12px;padding:10px 0;border-bottom:1px solid #f4f4f4;text-decoration:none;color:inherit}
.ib-row:hover .ib-subject{text-decoration:underline}
.ib-who{flex:0 0 150px;font-size:13px;color:#666;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ib-subject{flex:1;min-width:0;font-size:14px;color:#111;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ib-when{flex:none;font-size:11px;color:#bbb;white-space:nowrap}
.ib-new .ib-who,.ib-new .ib-subject{font-weight:600;color:#000}
.ib-tag{margin-left:8px;border:1px solid #eee;border-radius:999px;padding:1px 7px;font-size:10px;color:#777}
.ib-boxes{display:flex;flex-wrap:wrap;gap:6px;margin:0 0 14px}
.ib-box{border:1px solid #eee;border-radius:999px;padding:3px 11px;font-size:12px;color:#666;text-decoration:none}
.ib-box:hover{border-color:#ddd;color:#111}
.ib-box.on{background:#111;border-color:#111;color:#fff}
.ib-back{font-size:13px;margin:0 0 12px}
.ib-title{font-size:20px;margin:0 0 4px}
.ib-meta{font-size:12px;color:#999;margin:0 0 20px}
.ib-body{font-size:14px;line-height:1.7;white-space:pre-wrap;overflow-wrap:break-word}
@media (max-width:640px){.ib-who{flex-basis:100px}}
</style>`

// boxPath is a mailbox's own address.
func boxPath(box string) string {
	if box == "" {
		return "/inbox"
	}
	return "/inbox/" + box
}

// boxes is the switcher: every mailbox that has something in it, and All.
//
// An alias is a mailbox. asim+research@ goes to the research agent, so what
// arrives there is that agent's mail and not a slice of yours — the same shape
// as a mailbox per client, arrived at from the other direction. The tag rides
// on the message already; nothing new is stored to make this true.
//
// Derived from what has arrived rather than from the roster, and deliberately:
// this package must not import agent/, and a box that appears the moment it has
// mail is a truer statement than one that appears because an agent exists and
// has never been written to.
//
// Silent when there is only one, because a switcher with a single destination
// is a control that cannot do anything.
func boxes(all []*mail.Message, current string) string {
	seen := map[string]bool{}
	var tags []string
	for _, m := range all {
		if m.Tag == "" || seen[m.Tag] {
			continue
		}
		seen[m.Tag] = true
		tags = append(tags, m.Tag)
	}
	if len(tags) == 0 {
		return ""
	}
	sort.Strings(tags)

	chip := func(label, box string) string {
		cls := "ib-box"
		if strings.EqualFold(box, current) {
			cls += " on"
		}
		return `<a class="` + cls + `" href="` + boxPath(box) + `">` + html.EscapeString(label) + `</a>`
	}

	var b strings.Builder
	b.WriteString(`<div class="ib-boxes">` + chip("All", ""))
	for _, t := range tags {
		b.WriteString(chip(t, t))
	}
	b.WriteString(`</div>`)
	return b.String()
}
