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
// conversation when a client hands it over — see agent/mail — and this is the
// view over the conversations, not over the envelopes.
//
// Boxes are agents. An alias is an agent's address, so what arrives at
// you+research@ is the research agent's mail and /inbox/research is that box.
// The switcher is the reader's own mailboxes rather than a roster of somebody
// else's things.

import (
	"html"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/mail"
)

// shown is how many conversations one page of the inbox is.
const shown = 25

// labelChars is how much of an agent's name fits in the label column beside
// the channel it arrived on. Two pills wide, and the names people give agents
// are one word.
const labelChars = 14

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

// Agent is what this page needs to know about one of the account's agents:
// what to call it, and the alias its mail arrives at.
type Agent struct {
	ID   string
	Name string
	// Tag is the part after the plus. It is the agent's identity in an address
	// and therefore the name of its box — see roster.
	Tag string
}

// Agents is the account's roster, filled in by the server because this package
// may not import agent/. Nil on a build with no agents, which draws no switcher
// rather than an empty one.
var Agents func(owner string) []Agent

// roster is the account's agents, or none.
func roster(accountID string) []Agent {
	if Agents == nil || accountID == "" {
		return nil
	}
	return Agents(accountID)
}

// boxTag is the box an agent's mail belongs in, which is its address tag.
func boxTag(accountID, agentID string) string {
	if agentID == "" {
		return ""
	}
	for _, a := range roster(accountID) {
		if a.ID == agentID {
			return a.Tag
		}
	}
	return ""
}

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
	b.WriteString(addressBar(accountID, box))
	// What just happened, when something did. A message you sent appears in the
	// list below as a conversation, which is right and is also indistinguishable
	// from a message that failed to send — so the page says so once.
	if to := strings.TrimSpace(r.URL.Query().Get("sent")); to != "" {
		b.WriteString(`<p class="ib-sent">Sent to ` + html.EscapeString(trimTo(to, 80)) +
			`. Their reply lands on the same conversation.</p>`)
	}
	b.WriteString(boxes(accountID, all, box))

	// What is waiting to be let in, above the mailbox and only when there is
	// some. A held conversation is deliberately not in the list below, so
	// without this the difference between holding a stranger's message and
	// dropping it would be invisible from here.
	b.WriteString(waiting(r, accountID))

	// Search, over everything in the record rather than over the page.
	//
	// The inbox is where the conversations are, on every channel there is, and
	// there was no way to look for one. /recall could — thread.Search has been
	// there the whole time, exported and complete — but you had to know the
	// page existed and that it was the same store, which is two facts nothing
	// on this screen tells you. A mailbox you cannot search is a log.
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	b.WriteString(searchBox(box, q))
	if q != "" {
		found(&b, r, accountID, box, q)
		b.WriteString(`</div>`)
		app.Respond(w, r, app.Response{Title: "Inbox", Description: "What arrived", HTML: b.String()})
		return
	}

	if len(threads) == 0 {
		// An empty inbox says how to fill it, and the answer is an address.
		// "Nothing here" is a true sentence that leaves somebody looking at a
		// blank page with nothing to do about it. An empty box is a narrower
		// fact and gets the narrower sentence — the address is already above it.
		if box != "" {
			// The address, here rather than above every list.
			//
			// An empty box is the one place it is the answer: there is nothing
			// to read and the only useful thing to say is where to write so
			// there is. Printing it above a full mailbox was the version of
			// this that helped nobody.
			where := ""
			if alias := boxAddress(accountID, box); alias != "" {
				where = ` Write to ` + writeTo(alias) + ` and it turns up here.`
			}
			b.WriteString(`<p class="ib-empty">Nothing for <code>` + html.EscapeString(box) +
				`</code> yet.` + where + `</p>`)
		} else {
			// The address used to be printed directly above this and is not
			// any more, so the sentence says where to find it rather than
			// pointing at a line that has gone.
			b.WriteString(`<p class="ib-empty">Nothing has arrived yet. Write to your ` +
				`address from anywhere — your own mail, your phone — and it turns up here. ` +
				`The agent reads what arrives and answers in the thread.</p>` +
				`<p class="ib-empty">This is what came in. Chats you started here are with ` +
				`the agent, on ` + app.TextLink("Agents", "/agents") + `. Or ` +
				app.TextLink("write one yourself", "/inbox/new") + `.</p>`)
		}
		b.WriteString(`</div>`)
		app.Respond(w, r, app.Response{Title: "Inbox", Description: "What arrived", HTML: b.String()})
		return
	}

	pager := app.Paginate(r, len(threads), shown)
	for i := pager.From; i < pager.To; i++ {
		b.WriteString(row(r, accountID, threads[i]))
	}
	b.WriteString(pager.Nav(boxPath(box)))
	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: "Inbox", Description: "What arrived", HTML: b.String()})
}

// row is one conversation: who it is with, what it is about, the last thing
// said, and when. The shape of a mail client's list, because a list of
// conversations is what a mail client shows.
func row(r *http.Request, accountID string, t thread.Thread) string {
	return rowWith(r, accountID, t, "")
}

// rowWith is row, with the option of saying what to preview.
//
// Search needs it. The preview is normally the last thing said, which is the
// right answer for a list you are glancing down and the wrong one for a list of
// results: a search for "invoice" that shows the last line of each conversation
// makes you open every one to find out why it matched.
func rowWith(r *http.Request, accountID string, t thread.Thread, preview string) string {
	subject := strings.TrimSpace(t.Subject)
	if subject == "" {
		subject = "Untitled"
	}

	// Who the conversation is *with*, which is not who spoke last.
	//
	// It was the last speaker, so an inbound email from Henrik was labelled
	// "Agent" the moment the agent answered it — the row said the message was
	// from an agent when it was from a person, and the whole list relabelled
	// itself as replies landed. No mail client does that: the first column is
	// who you are corresponding with, and it does not change because the last
	// word happened to be yours.
	who, full := party(accountID, t)
	// The preview, with the subject taken off the front of it. Mail recorded
	// before thread.Name existed has the subject inside the message, so the
	// preview read "Invoice 4021 Attached is this month's…" — the subject
	// twice, once as the subject. See withoutSubject.
	// And with the quoted tail off it too, so a reply that says "Yes, do that"
	// above three exchanges of history previews as "Yes, do that" — see
	// quoted.go.
	snippet := ""
	if preview != "" {
		snippet = trimTo(strings.TrimSpace(withoutSubject(preview, subject)), 110)
	} else if msgs := thread.Messages(accountID, t.ID, 1); len(msgs) > 0 {
		text, _ := unquoted(withoutSubject(msgs[0].Text, subject))
		snippet = trimTo(text, 110)
	}

	// Who, where from, and when — on their own line above the subject.
	//
	// These have been three arrangements and each broke for the same reason:
	// they were competing with the subject for one line. Beside it they pushed
	// the subject off; in a fixed column they left a ragged gap on every row
	// that had one label instead of two; as pills they were two boxes of
	// chrome in the middle of a sentence. On a phone there was never room for
	// any of it.
	//
	// A row is three lines now, which is what a mail client on a phone has
	// always been: who it is from and when, then what it is about, then the
	// first of what it says. Nothing competes, because nothing shares a line.
	//
	// Plain text separated by middots rather than pills. A pill is for a thing
	// you can act on; the channel a message arrived by is a fact about it, and
	// two facts in boxes read as two buttons.
	meta := []string{html.EscapeString(who)}
	if c := app.ClientName(t.Client); c != "" {
		meta = append(meta, html.EscapeString(c))
	}
	if name := agentLabel(accountID, t.Agent); name != "" {
		meta = append(meta, html.EscapeString(trimTo(name, labelChars)))
	}
	meta = append(meta, html.EscapeString(app.TimeAgo(t.Updated)))

	// Unread, which is what makes this a mailbox rather than a log. Without it
	// every row looks the same and the page has to be read top to bottom every
	// time, because nothing says which of these you have dealt with.
	cls := "ib-row"
	if thread.Unread(t) {
		cls += " unseen"
	}

	// Delete, on the row.
	//
	// It was only on the conversation, so throwing away a thread you can see is
	// junk meant opening it — which marks it read on the way in, and reading
	// something in order to discard it is the one interaction a mailbox exists
	// to save you. Every mail client puts it on the row for that reason.
	//
	// Beside the link rather than inside it: a form cannot live in an <a>, and
	// nesting a submit inside a navigation target means a click has two
	// meanings. So the row is a flex pair — the link, which fills it, and this.
	return `<div class="ib-item">` +
		`<a class="` + cls + `" href="/inbox?id=` + url.QueryEscape(t.ID) + `"` + titleAttr(full) + `>` +
		`<span class="ib-meta">` + strings.Join(meta, `<span class="ib-dot">·</span>`) + `</span>` +
		`<span class="ib-subject">` + html.EscapeString(trimTo(subject, 90)) + `</span>` +
		`<span class="ib-snip">` + html.EscapeString(snippet) + `</span></a>` +
		rowDelete(r, t.ID) + `</div>`
}

// rowDelete is the cross at the end of a row.
//
// A glyph rather than the word, because it repeats down the page and twenty
// "Delete"s is a column of warnings. It is labelled for anything not reading
// the shape, and it asks first — thread.Delete is not recoverable.
func rowDelete(r *http.Request, id string) string {
	return `<form class="ib-del" method="post" action="/inbox/delete" ` +
		`onsubmit="return confirm('Delete this conversation? What was said in it is gone.')">` +
		`<input type="hidden" name="id" value="` + html.EscapeString(id) + `">` +
		`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<button class="ib-del-go" type="submit" title="Delete" aria-label="Delete this conversation">` +
		`&times;</button></form>`
}

// party is who a conversation is with, and the address behind the name.
//
// The other people on it — anybody who is not this account and not an agent.
// "You" when there is nobody else, which is every conversation you started
// yourself: your own name in your own inbox is furniture, and "You · What is
// the weather" reads correctly as a note to yourself.
//
// Names, not addresses, because the address is on the hover and in the
// conversation. Two people are joined with a comma the way a mail client does
// it; more than two is counted, because three names is wider than the column.
func party(accountID string, t thread.Thread) (who, full string) {
	var names, addrs []string
	for _, p := range thread.Parties(accountID, t.ID) {
		if p.Kind == thread.RoleAgent || p.Key == "" {
			continue
		}
		if strings.EqualFold(p.Key, accountID) {
			continue
		}
		name := p.Name
		if name == "" {
			name = senderName(accountID, t.ID, p.Key)
		}
		names = append(names, trimTo(name, 22))
		addrs = append(addrs, p.Key)
	}
	switch len(names) {
	case 0:
		return "You", ""
	case 1:
		return names[0], addrs[0]
	case 2:
		return names[0] + ", " + names[1], strings.Join(addrs, ", ")
	}
	return names[0] + " +" + strconv.Itoa(len(names)-1), strings.Join(addrs, ", ")
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
	// The same width as the list. It was wider to hold a second column, and
	// there is no second column.
	b.WriteString(`<div class="ib">`)
	// Where you came from, and what you can do to this — one bar rather than
	// three loose things stacked above the conversation. See app.Actions.
	b.WriteString(app.Actions(app.TextLink("← Inbox", "/inbox"),
		unreadButton(r, t.ID, wasUnread), deleteButton(r, t.ID)))

	// One column, and the agent's answers in it.
	//
	// There were two: the correspondence on the left and a chat with the agent
	// on the right, with split() pulling the agent's messages out of the thread
	// to fill the second one. Both are gone.
	//
	// The argument for the split was that a mail thread read badly with your own
	// instructions interleaved through it. That was true of the control they
	// were interleaved by — a box you typed in and waited at, which is a chat,
	// on the page that exists precisely so you do not have to wait. With the
	// waiting gone there is nothing to hold apart: you hand the thing over and
	// the answer arrives later as a message, which is what every other message
	// on this thread is.
	//
	// So the conversation gets the width back, and the agent is told who a reply
	// would go to so its caption can point at the Reply button rather than only
	// saying what it is not.
	msgs := thread.Messages(accountID, t.ID, MessagesShown)
	b.WriteString(conversationPane(accountID, t, msgs, len(msgs) >= MessagesShown, false,
		assignDialog(r, accountID, t, replyTo(accountID, t, msgs))))
	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{Title: subject, Description: "A conversation", HTML: b.String()})
}

// boxOfThread is which mailbox a conversation belongs in: the agent it is with.
func boxOfThread(accountID string, t thread.Thread) string {
	return boxTag(accountID, t.Agent)
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

// boxPath is a mailbox's own address.
func boxPath(box string) string {
	if box == "" {
		return "/inbox"
	}
	return "/inbox/" + box
}

// boxes is the switcher: one per agent, and All.
//
// It was derived from what had arrived, on the argument that "a box that
// appears the moment it has something in it is a truer statement than one that
// appears because an agent exists and has never been written to". That was
// right while a box was only a filter over this list, and it stopped being
// right when the box began carrying the agent's address: the agent you have
// never written to is exactly the one whose address you need, and it was the
// one with no box to select.
//
// So it is the roster, and a box with nothing in it says so rather than being
// missing. The other half of the old reason — that this package may not import
// agent/ — is answered by the hook, the same way AgentName already was.
//
// Silent when there are no agents, because a switcher with one destination is a
// control that cannot do anything.
func boxes(accountID string, all []thread.Thread, current string) string {
	agents := roster(accountID)
	if len(agents) == 0 {
		return ""
	}

	// Newest first is how the roster comes back, which is an order for a page
	// about making agents rather than one about reading their mail. Here they
	// are a row of chips somebody scans for a name.
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })

	chip := func(label, box string) string {
		return app.PillLink(label, boxPath(box), strings.EqualFold(box, current))
	}

	// Say what the row is.
	//
	// It was a row of names with nothing above it, and a name on a chip does
	// not say whether it filters, navigates or addresses. These are the account's
	// mailboxes — one per agent, each with its own address — so the word is
	// Mailboxes, which is what the rail already calls them (see Mailboxes) and
	// what a person coming from any mail client already knows.
	var b strings.Builder
	b.WriteString(`<p class="home-section ib-boxes-head"><small>Mailboxes</small></p>`)
	b.WriteString(`<div class="ib-boxes">` + chip("All", ""))
	for _, a := range agents {
		if a.Tag == "" {
			continue // no alias, so nothing arrives at one: it is only in All
		}
		label := strings.TrimSpace(a.Name)
		if label == "" {
			label = a.Tag
		}
		b.WriteString(chip(label, a.Tag))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// addressBar is the two addresses this page is about.
//
// It showed one — the agent's — and then New sent as a different one, with
// nothing saying why. Both are real and they are for different things, and the
// order matters: yours first, because this is your inbox and the agent is in
// it rather than the other way round.
//
//	you@       mail to you lands here, and this is what New sends as
//	agent@     write to it and it answers, in the thread
//
// Same page, because they arrive in the same place. A stranger writing to your
// address and a stranger writing to your agent are both things that turned up
// while you were elsewhere, which is what this page is.
//
// # The agent address follows the box
//
// A box is an agent — /inbox/research is what arrived at you+research@ — and
// this showed the instance agent's address on every one of them. So the
// switcher above changed which mail you were looking at and the line above it
// went on naming a different agent, which is the address you would have copied.
// It takes the box now: All shows the instance agent, and a named box shows the
// alias that reaches it.
//
// And the address is a link into New with it already filled in, because
// "write to the agent" is what somebody reading this line is trying to do and
// the alternative was copying it by hand into a form two clicks away.
func addressBar(accountID, box string) string {
	var b strings.Builder

	// The two controls, and nothing else.
	//
	// This printed "You asim@micro.mu / Agent agent@micro.mu / IMAP" above every
	// list once. Three facts, none of them what somebody opening their inbox
	// came to find out, and they were cut back to one sentence — "Everything
	// sent to you, on every channel" — which has the same problem in fewer
	// words: it is the page's own title said again, read once and then read
	// every visit afterwards on the way past.
	//
	// The addresses have not gone. The agent's is filled in for you on New,
	// which is where you would use it rather than copy it, and both are on
	// /inbox/imap with everything else a client asks for.
	//
	// Buttons rather than pills, on the left rather than out to the right. They
	// are the page's actions and every other page in this product puts those at
	// the top left in that shape; two pills floated right were this page's own
	// arrangement and nowhere else's.
	acts := ""
	if mail.Reachable() {
		acts = app.ActionLink("/inbox/new", "New")
	}
	// No Connect a mail client here.
	//
	// It sat beside New as a text link, on the reasoning that writing a message
	// is what somebody does from here and setting up a mail client is something
	// they do once, if ever. That reasoning is the argument for taking it off
	// the page rather than for shrinking it: a control used once in the life of
	// an account does not belong on the screen its owner opens every day, where
	// it is read past several thousand times to be used never again. /inbox/imap
	// still exists and /account is where a thing you set up once belongs.
	b.WriteString(`<div class="page-action ib-acts">` + acts + `</div>`)
	return b.String()
}

// boxAddress is the alias that reaches the agent whose box this is.
//
// mail.Handle rather than accountID + "+" + box, which is the same string until
// it is not: Handle cleans the tag by the service's own rule, and the service is
// what decides which addresses it will accept.
func boxAddress(accountID, box string) string {
	if box == "" {
		return ""
	}
	return mail.EmailForUser(mail.Handle(accountID, box), mail.ConfiguredDomain())
}

// No howTo, and the reasoning that put it here is the reasoning against it.
//
// It was four numbered lines above the filters: write to the address, make a
// task, Cc an agent, connect IMAP. The argument was that the parts of this page
// which are not a mailbox are invisible until somebody tries them, so the page
// should say so — quiet, above the fold, read once and then never again.
//
// The half of that which is true is that it is read once. The half that is not
// is "and then never again": it was rendered on every load of the inbox, for
// everybody, forever, so the cost is paid by every reader on every visit and
// the benefit lands on one reader once. Three of the four lines pointed at
// other pages, which is a table of contents for the product printed at the top
// of the mail.
//
// The address bar above still says what this page is for and links to the
// agents. That is the orientation this needed.

// writeTo is an address you can write to: the address, and a click that opens
// New with it in the To box.
//
// Not a button beside it. The address is the thing on the page a reader is
// already looking at when they decide to write, and a second control next to it
// asks them to notice two things where there is one.
func writeTo(addr string) string {
	code := `<code>` + html.EscapeString(addr) + `</code>`
	if !mail.Reachable() {
		return code
	}
	return `<a class="ib-addr-write" href="/inbox/new?to=` +
		html.EscapeString(url.QueryEscape(addr)) + `" title="Write to ` +
		html.EscapeString(addr) + `">` + code + `</a>`
}

// Mailboxes is the rail's view of this account's boxes: All, and one per agent
// that has something in it. The same list the switcher draws, so the sidebar
// and the page cannot disagree about what boxes exist.
func Mailboxes(accountID string) []app.NavItem {
	if accountID == "" {
		return nil
	}
	agents := roster(accountID)
	if len(agents) == 0 {
		return nil
	}

	unread := map[string]int{} // tag -> how many, "" for the whole inbox
	for _, t := range arrivals(accountID) {
		if !thread.Unread(t) {
			continue
		}
		unread[""]++
		if box := boxOfThread(accountID, t); box != "" {
			unread[box]++
		}
	}

	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })

	badge := func(n int) string {
		if n == 0 {
			return ""
		}
		return app.Count(n)
	}

	out := []app.NavItem{{Label: "All", Href: "/inbox", Badge: badge(unread[""])}}
	for _, a := range agents {
		if a.Tag == "" {
			continue
		}
		label := strings.TrimSpace(a.Name)
		if label == "" {
			label = a.Tag
		}
		out = append(out, app.NavItem{Label: label, Href: boxPath(a.Tag), Key: a.Tag,
			Badge: badge(unread[a.Tag])})
	}
	return out
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
