package inbox

// The inbox: a mail client over the record.
//
// It reads internal/thread — every conversation this account has had, on
// whichever client it arrived — and renders it the way a mail client renders a
// mailbox: a row per conversation, who last spoke, what it is about, the last
// thing said, when. That is what makes it one inbox rather than five. An email
// chain, a WhatsApp exchange and a chat on this page are the same kind of thing
// in the record, and the only reason they ever looked like different things is
// that they used to be kept in different places. They are not.
//
// Not the mail store. service/mail is the MTA and holds what SMTP delivered;
// /mail is its page and this does not touch it. A message becomes a
// conversation when a client hands it over — see client/mail — and this is the
// view over the conversations, not over the envelopes.
//
// Boxes are agents. An alias is an agent's address, so what arrives at
// you+research@ is the research agent's mail and /inbox/research is that box.
// The switcher is the reader's own mailboxes rather than a roster of somebody
// else's things, which is why it does not break the rule in DIRECTION §8.

import (
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// shown is how many conversations one page of the inbox is.
const shown = 25

// held is how far back the list reaches. Going further back on purpose is what
// /recall is for — a mailbox is somewhere you glance, not a search index.
const held = 500

// AgentName is what to call the agent a conversation is with, filled in by the
// agent package because the roster is its own. Empty for an agent that is no
// longer here — see agentLabel.
var AgentName func(owner, id string) string

// Address is where mail for this instance's agent arrives, filled in by the
// server. A hook rather than an import because one string is not a reason for
// this package to depend on the mail service.
var Address func() string

// Handler serves /inbox and /inbox/<box>.
func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if id := r.URL.Query().Get("id"); id != "" {
		conversation(w, r, acc.ID, id)
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
	return box
}

// list is the inbox proper.
func list(w http.ResponseWriter, r *http.Request, accountID, box string) {
	all := thread.List(accountID, held)

	threads := all
	if box != "" {
		threads = nil
		for _, t := range all {
			if strings.EqualFold(boxOfThread(accountID, t), box) {
				threads = append(threads, t)
			}
		}
	}

	var b strings.Builder
	b.WriteString(`<div class="ib">`)
	b.WriteString(addressBar())
	b.WriteString(boxes(accountID, all, box))

	if len(threads) == 0 {
		// An empty inbox says how to fill it, and the answer is an address.
		// "Nothing here" is a true sentence that leaves somebody looking at a
		// blank page with nothing to do about it. An empty box is a narrower
		// fact and gets the narrower sentence — the address is already above it.
		if box != "" {
			b.WriteString(`<p class="ib-empty">Nothing for <code>` + html.EscapeString(box) +
				`</code> yet.</p>`)
		} else {
			b.WriteString(`<p class="ib-empty">Nothing here yet. Write to the address above from ` +
				`anywhere — your own mail, your phone — and it turns up here. The agent reads ` +
				`what arrives and answers in the thread.</p>`)
		}
		b.WriteString(`</div>` + inboxCSS)
		app.Respond(w, r, app.Response{Title: "Inbox", Description: "What arrived", HTML: b.String()})
		return
	}

	pager := app.Paginate(r, len(threads), shown)
	for i := pager.From; i < pager.To; i++ {
		b.WriteString(row(accountID, threads[i]))
	}
	b.WriteString(pager.Nav(boxPath(box)))
	b.WriteString(`</div>` + inboxCSS)

	app.Respond(w, r, app.Response{Title: "Inbox", Description: "What arrived", HTML: b.String()})
}

// row is one conversation: who last spoke, what it is about, the last thing
// said, and when. The shape of a mail client's list, because a list of
// conversations is what a mail client shows.
func row(accountID string, t thread.Thread) string {
	subject := strings.TrimSpace(t.Subject)
	if subject == "" {
		subject = "Untitled"
	}

	who, snippet := "You", ""
	if msgs := thread.Messages(accountID, t.ID, 1); len(msgs) > 0 {
		m := msgs[0]
		switch {
		case m.Role == thread.RoleAgent:
			who = "Agent"
		case m.From != "":
			who = m.From
		}
		snippet = trimTo(m.Text, 110)
	}

	// Where it happened, when that is not here. A row saying "Web" on the page
	// you are already on is noise; one saying "Email" is the fact worth showing,
	// because it means the conversation happened without you.
	where := ""
	if t.Client != thread.WebClient {
		where = `<span class="ib-tag">` + html.EscapeString(app.ClientName(t.Client)) + `</span>`
	}

	return `<a class="ib-row" href="/inbox?id=` + url.QueryEscape(t.ID) + `">` +
		`<span class="ib-who">` + html.EscapeString(trimTo(who, 24)) + `</span>` +
		`<span class="ib-mid"><span class="ib-subject">` +
		html.EscapeString(trimTo(subject, 70)) + `</span>` + where +
		`<span class="ib-snip">` + html.EscapeString(snippet) + `</span></span>` +
		`<span class="ib-when">` + html.EscapeString(app.TimeAgo(t.Updated)) + `</span></a>`
}

// conversation is one thread, read.
func conversation(w http.ResponseWriter, r *http.Request, accountID, id string) {
	t := thread.Get(accountID, id)
	if t == nil {
		// Scoped to the reader by thread.Get, so somebody else's id is not
		// "forbidden" — it is not a thing that exists here.
		app.NotFound(w, r, "no conversation here with that id")
		return
	}

	subject := strings.TrimSpace(t.Subject)
	if subject == "" {
		subject = "Untitled"
	}

	var b strings.Builder
	b.WriteString(`<div class="ib">`)
	b.WriteString(`<p class="ib-back">` + app.TextLink("← Inbox", "/inbox") + `</p>`)
	b.WriteString(ConversationView(accountID, t))
	b.WriteString(`</div>` + inboxCSS)

	app.Respond(w, r, app.Response{Title: subject, Description: "A conversation", HTML: b.String()})
}

// boxOfThread is which mailbox a conversation belongs in: the agent it is with.
func boxOfThread(accountID string, t thread.Thread) string {
	if t.Agent == "" {
		return ""
	}
	return slugOf(agentLabel(accountID, t.Agent))
}

// agentLabel is what to call an agent, and empty when there is nothing to call
// it.
//
// The id used to be the fallback, on the reasoning that a box named badly beats
// a box that vanishes. That was wrong in both halves. An id that resolves to no
// agent means the agent has been deleted, so there is no box to lose — the
// conversations are still in All, which is where they belong once the thing they
// were with is gone. And what it actually produced was a rail listing
// "47b6428c-fa8a-4610-a302-45dbc992ad5d" as though that were somewhere to click:
// three of four mailboxes named after rows in a file.
func agentLabel(accountID, id string) string {
	if AgentName == nil {
		return ""
	}
	return strings.TrimSpace(AgentName(accountID, id))
}

// slugOf is a box's name in a path — lowercase, letters and digits only, the
// same shape an agent's own address tag takes, so /inbox/research and
// you+research@ are the same word.
func slugOf(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// boxPath is a mailbox's own address.
func boxPath(box string) string {
	if box == "" {
		return "/inbox"
	}
	return "/inbox/" + box
}

// boxes is the switcher: one per agent that has a conversation, and All.
//
// Derived from what has arrived rather than from the roster. A box that appears
// the moment it has something in it is a truer statement than one that appears
// because an agent exists and has never been written to — and this package does
// not import agent/, which is the other half of the reason.
//
// Silent when there is only one, because a switcher with a single destination
// is a control that cannot do anything.
func boxes(accountID string, all []thread.Thread, current string) string {
	seen := map[string]string{} // slug -> label
	for _, t := range all {
		if t.Agent == "" {
			continue
		}
		label := agentLabel(accountID, t.Agent)
		if s := slugOf(label); s != "" {
			seen[s] = label
		}
	}
	if len(seen) == 0 {
		return ""
	}
	slugs := make([]string, 0, len(seen))
	for s := range seen {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)

	chip := func(label, box string) string {
		cls := "ib-box"
		if strings.EqualFold(box, current) {
			cls += " on"
		}
		return `<a class="` + cls + `" href="` + boxPath(box) + `">` + html.EscapeString(label) + `</a>`
	}

	var b strings.Builder
	b.WriteString(`<div class="ib-boxes">` + chip("All", ""))
	for _, s := range slugs {
		b.WriteString(chip(seen[s], s))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// addressBar is what makes this an agentic inbox rather than a mailbox: the
// address the agent answers at, on the page where its mail lands.
func addressBar() string {
	if Address == nil {
		return ""
	}
	addr := Address()
	if addr == "" {
		return ""
	}
	return `<div class="ib-addr"><code>` + html.EscapeString(addr) + `</code>` +
		`<span class="ib-addr-note">Write here from anywhere and the agent answers in the thread. ` +
		app.TextLink("Your agents", "/agents") + `</span></div>`
}

const inboxCSS = `<style>
.ib{max-width:760px}
.ib-addr{display:flex;flex-wrap:wrap;align-items:baseline;gap:10px;margin:0 0 16px;padding-bottom:14px;border-bottom:1px solid #eee}
.ib-addr code{font-size:13px;background:#f5f5f5;border-radius:4px;padding:3px 8px}
.ib-addr-note{font-size:12px;color:#999}
.ib-boxes{display:flex;flex-wrap:wrap;gap:6px;margin:0 0 14px}
.ib-box{border:1px solid #eee;border-radius:999px;padding:3px 11px;font-size:12px;color:#666;text-decoration:none}
.ib-box:hover{border-color:#ddd;color:#111}
.ib-box.on{background:#111;border-color:#111;color:#fff}
.ib-empty{font-size:14px;color:#888;line-height:1.6}
.ib-row{display:flex;align-items:baseline;gap:12px;padding:11px 0;border-bottom:1px solid #f4f4f4;text-decoration:none;color:inherit}
.ib-row:hover .ib-subject{text-decoration:underline}
.ib-who{flex:0 0 130px;font-size:13px;font-weight:500;color:#111;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ib-mid{flex:1;min-width:0;display:flex;align-items:baseline;gap:8px;overflow:hidden}
.ib-subject{font-size:14px;color:#111;white-space:nowrap;flex:none;max-width:60%;overflow:hidden;text-overflow:ellipsis}
.ib-snip{font-size:13px;color:#999;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ib-when{flex:none;font-size:11px;color:#bbb;white-space:nowrap}
.ib-tag{border:1px solid #eee;border-radius:999px;padding:1px 7px;font-size:10px;color:#777;flex:none}
.ib-back{font-size:13px;margin:0 0 12px}
@media (max-width:640px){
  .ib-who{flex-basis:90px}
  .ib-snip{display:none}
}
</style>`

// Mailboxes is the rail's view of this account's boxes: All, and one per agent
// that has something in it. The same list the switcher draws, so the sidebar
// and the page cannot disagree about what boxes exist.
func Mailboxes(accountID string) []app.NavItem {
	if accountID == "" {
		return nil
	}
	all := thread.List(accountID, held)
	seen := map[string]string{}
	for _, t := range all {
		if t.Agent == "" {
			continue
		}
		if s := slugOf(agentLabel(accountID, t.Agent)); s != "" {
			seen[s] = agentLabel(accountID, t.Agent)
		}
	}
	if len(seen) == 0 {
		return nil
	}
	slugs := make([]string, 0, len(seen))
	for s := range seen {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)

	out := []app.NavItem{{Label: "All", Href: "/inbox"}}
	for _, s := range slugs {
		out = append(out, app.NavItem{Label: seen[s], Href: boxPath(s), Key: s})
	}
	return out
}
