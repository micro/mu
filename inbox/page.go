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
	"mu/service/mail"
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
	// An instruction about the conversation being read. POST here rather than at
	// a path of its own, because /inbox/<box> is a mailbox name and /inbox/act
	// would be one an account could have.
	if r.Method == http.MethodPost {
		action(w, r, acc.ID)
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

// arrivals is what belongs in the inbox: the conversations that came in,
// whichever channel carried them.
//
// Not the chats you started here. Those are on /agent, which is where you were
// sitting when you had them — see thread.Arrived. Without this line the two
// pages are two lists of the same conversations with different furniture, which
// is what they were, and neither could be described in a sentence.
func arrivals(accountID string) []thread.Thread {
	all := thread.List(accountID, held)
	out := all[:0:0]
	for _, t := range all {
		if thread.Arrived(t) {
			out = append(out, t)
		}
	}
	return out
}

// list is the inbox proper.
func list(w http.ResponseWriter, r *http.Request, accountID, box string) {
	all := arrivals(accountID)

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
	b.WriteString(addressBar(accountID))
	// What just happened, when something did. A message you sent appears in the
	// list below as a conversation, which is right and is also indistinguishable
	// from a message that failed to send — so the page says so once.
	if to := strings.TrimSpace(r.URL.Query().Get("sent")); to != "" {
		b.WriteString(`<p class="ib-sent">Sent to ` + html.EscapeString(trimTo(to, 80)) +
			`. Their reply lands on the same conversation.</p>`)
	}
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
			b.WriteString(`<p class="ib-empty">Nothing has arrived yet. Write to the address ` +
				`above from anywhere — your own mail, your phone — and it turns up here. The ` +
				`agent reads what arrives and answers in the thread.</p>` +
				`<p class="ib-empty">This is what came in. Chats you started here are with ` +
				`the agent, on ` + app.TextLink("Agents", "/agents") + `. Or ` +
				app.TextLink("write one yourself", "/inbox/compose") + ` — the agent will draft it.</p>`)
		}
		b.WriteString(`</div>`)
		app.Respond(w, r, app.Response{Title: "Inbox", Description: "What arrived", HTML: b.String()})
		return
	}

	pager := app.Paginate(r, len(threads), shown)
	for i := pager.From; i < pager.To; i++ {
		b.WriteString(row(accountID, threads[i]))
	}
	b.WriteString(pager.Nav(boxPath(box)))
	b.WriteString(`</div>`)

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

	who, full, snippet := "You", "", ""
	if msgs := thread.Messages(accountID, t.ID, 1); len(msgs) > 0 {
		m := msgs[0]
		switch {
		case m.Role == thread.RoleAgent:
			who = "Agent"
		case m.From != "":
			who, full = senderName(accountID, t.ID, m.From), m.From
		}
		snippet = trimTo(m.Text, 110)
	}

	// Which channel carried it. Every row here arrived from somewhere that is
	// not this page, so the label is always worth showing — it is the answer to
	// "where do I reply".
	where := app.Pill(app.ClientName(t.Client))

	// Unread, which is what makes this a mailbox rather than a log. Without it
	// every row looks the same and the page has to be read top to bottom every
	// time, because nothing says which of these you have dealt with.
	cls := "ib-row"
	if thread.Unread(t) {
		cls += " unseen"
	}

	return `<a class="` + cls + `" href="/inbox?id=` + url.QueryEscape(t.ID) + `">` +
		`<span class="ib-who"` + titleAttr(full) + `>` + html.EscapeString(who) + `</span>` +
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

	// Opening it is reading it. Before rendering, so a reload of the page you
	// are already on does not still show it bold.
	wasUnread := thread.Unread(*t)
	thread.MarkSeen(accountID, t.ID)

	var b strings.Builder
	b.WriteString(`<div class="ib">`)
	// Where you came from, and what you can do to this — one bar rather than
	// three loose things stacked above the conversation. See app.Actions.
	b.WriteString(app.Actions(app.TextLink("← Inbox", "/inbox"),
		unreadButton(r, t.ID, wasUnread), deleteButton(r, t.ID)))
	b.WriteString(ConversationView(accountID, t))
	// The agent, on the thing you are reading. See act.go.
	b.WriteString(askBox(r, t.ID))
	b.WriteString(`</div>`)

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
		return app.PillLink(label, boxPath(box), strings.EqualFold(box, current))
	}

	var b strings.Builder
	b.WriteString(`<div class="ib-boxes">` + chip("All", ""))
	for _, s := range slugs {
		b.WriteString(chip(seen[s], s))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// addressBar is the two addresses this page is about.
//
// It showed one — the agent's — and then compose sent as a different one, with
// nothing saying why. Both are real and they are for different things, and the
// order matters: yours first, because this is your inbox and the agent is in
// it rather than the other way round.
//
//	you@       mail to you lands here, and this is what compose sends as
//	agent@     write to it and it answers, in the thread
//
// Same page, because they arrive in the same place. A stranger writing to your
// address and a stranger writing to your agent are both things that turned up
// while you were elsewhere, which is what this page is.
func addressBar(accountID string) string {
	mine := mail.EmailForUser(accountID, mail.ConfiguredDomain())
	theirs := ""
	if Address != nil {
		theirs = Address()
	}
	if mine == "" && theirs == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<div class="ib-addr">`)
	if mine != "" {
		b.WriteString(`<span class="ib-addr-one"><span class="ib-addr-k">You</span>` +
			`<code>` + html.EscapeString(mine) + `</code></span>`)
	}
	if theirs != "" && !strings.EqualFold(theirs, mine) {
		b.WriteString(`<span class="ib-addr-one"><span class="ib-addr-k">Agent</span>` +
			`<code>` + html.EscapeString(theirs) + `</code></span>`)
	}
	b.WriteString(`<span class="ib-addr-note">Mail to either lands here. Write to the agent ` +
		`from anywhere and it answers in the thread. ` + app.TextLink("Your agents", "/agents") +
		`</span>` + composeLink() + `</div>`)
	return b.String()
}

// Mailboxes is the rail's view of this account's boxes: All, and one per agent
// that has something in it. The same list the switcher draws, so the sidebar
// and the page cannot disagree about what boxes exist.
func Mailboxes(accountID string) []app.NavItem {
	if accountID == "" {
		return nil
	}
	all := arrivals(accountID)
	seen := map[string]string{} // slug -> label
	unread := map[string]int{}  // slug -> how many, "" for the whole inbox
	for _, t := range all {
		box := ""
		if t.Agent != "" {
			label := agentLabel(accountID, t.Agent)
			if s := slugOf(label); s != "" {
				seen[s], box = label, s
			}
		}
		if thread.Unread(t) {
			unread[""]++
			if box != "" {
				unread[box]++
			}
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

	badge := func(n int) string {
		if n == 0 {
			return ""
		}
		return app.Count(n)
	}

	out := []app.NavItem{{Label: "All", Href: "/inbox", Badge: badge(unread[""])}}
	for _, s := range slugs {
		out = append(out, app.NavItem{Label: seen[s], Href: boxPath(s), Key: s,
			Badge: badge(unread[s])})
	}
	return out
}

// Unread is how many conversations have arrived and not been read, for the
// sidebar. Only arrivals: a chat you had here five minutes ago is not something
// waiting for you.
func Unread(accountID string) int {
	n := 0
	for _, t := range arrivals(accountID) {
		if thread.Unread(t) {
			n++
		}
	}
	return n
}

// senderName is what to call whoever wrote, in a column 130px wide.
//
// The address is what a message carries and it is the wrong thing to show: a
// list of "henrik@getdirectree.co…" tells you nothing that "henrik" does not,
// and the part that got cut off is the part that would have. So the display
// name where the conversation knows one, the local part otherwise, and the
// whole address in a title attribute for anybody who wants it.
func senderName(accountID, threadID, addr string) string {
	for _, p := range thread.Parties(accountID, threadID) {
		if p.Kind == thread.RolePerson && strings.EqualFold(p.Key, addr) && p.Name != "" {
			return trimTo(p.Name, 22)
		}
	}
	if local, _, ok := strings.Cut(addr, "@"); ok && local != "" {
		return trimTo(local, 22)
	}
	return trimTo(addr, 22)
}

// titleAttr is a hover label, and nothing at all when there is nothing to say.
func titleAttr(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return ` title="` + html.EscapeString(s) + `"`
}
