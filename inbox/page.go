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
	b.WriteString(howTo())
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
	if msgs := thread.Messages(accountID, t.ID, 1); len(msgs) > 0 {
		text, _ := unquoted(withoutSubject(msgs[0].Text, subject))
		snippet = trimTo(text, 110)
	}

	// The labels, before the subject rather than after it.
	//
	// They were between the subject and the snippet, which is the one place
	// they cannot be read: mid-sentence, in the middle of the row. A label is
	// how you decide whether to read the line at all, so it comes first.
	//
	// Which channel carried it — every row here arrived from somewhere that is
	// not this page, so it answers "where do I reply" — and which agent it is
	// with, where it is with one. "Agent" as a bare word was the other half of
	// the confusion: it named a role rather than saying which of them, and
	// there are eleven.
	// In a column of their own, so every subject on the page starts at the same
	// place. They were inline before the subject, and a label is one pill or two
	// and "Here" or "WhatsApp" wide — so the subject began somewhere different
	// on every row and the eye had nothing to run down.
	//
	// Trimmed, because the column only holds so much and a name clipped
	// mid-pill looks like a rendering fault rather than a long name.
	labels := app.Pill(app.ClientName(t.Client))
	if name := agentLabel(accountID, t.Agent); name != "" {
		labels += app.Pill(trimTo(name, labelChars))
	}
	labels = `<span class="ib-tags">` + labels + `</span>`

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
		`<a class="` + cls + `" href="/inbox?id=` + url.QueryEscape(t.ID) + `">` +
		`<span class="ib-who"` + titleAttr(full) + `>` + html.EscapeString(who) + `</span>` +
		// The labels and the subject are one line, and the preview is the next.
		//
		// They were three siblings of .ib-mid, which is a flex row on a wide
		// screen and a column on a phone — so on a phone each got a line of its
		// own and the labels were pushed under the preview by an order:3 that
		// existed to stop them being first. A label is how you decide whether to
		// read the line, so it belongs beside what it labels on every width.
		`<span class="ib-mid"><span class="ib-line">` + labels +
		`<span class="ib-subject">` + html.EscapeString(trimTo(subject, 70)) +
		`</span></span>` +
		`<span class="ib-snip">` + html.EscapeString(snippet) + `</span></span>` +
		`<span class="ib-when">` + html.EscapeString(app.TimeAgo(t.Updated)) + `</span></a>` +
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
		assignDialog(r, t.ID, replyTo(accountID, t, msgs))))
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

	var b strings.Builder
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
	mine := mail.EmailForUser(accountID, mail.ConfiguredDomain())

	// The agent this box belongs to. A named box is the account's own alias for
	// it; All is the instance's agent, which is the one a stranger can reach.
	//
	// mail.Handle rather than accountID + "+" + box, which is the same string
	// until it is not: Handle cleans the tag by the service's own rule, and the
	// service is what decides which addresses it will accept.
	theirs := ""
	if box != "" {
		theirs = mail.EmailForUser(mail.Handle(accountID, box), mail.ConfiguredDomain())
	} else if Address != nil {
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
			writeTo(theirs) + `</span>`)
	}
	b.WriteString(`<span class="ib-addr-note">Work with agents from your inbox. ` +
		app.TextLink("Your agents", "/agents") +
		`</span>` + newLink() + `</div>`)
	return b.String()
}

// howTo is what to do with this page, in four lines.
//
// The address bar above says "Work with agents from your inbox", which is a
// claim rather than an instruction: it tells somebody what the page is for and
// nothing about how. Everything underneath is a list of conversations, and a
// list of conversations teaches you nothing you did not already know about
// mailboxes — the parts that are not a mailbox (an agent answers, Cc works,
// each agent is a folder, a client can open it) are invisible until somebody
// tries them.
//
// Quiet, and above the filters rather than below them. It is orientation, which
// is read once and then never again, so it must not compete with the mail: same
// muted grey the row snippets use, numbers rather than bullets because these
// are four separate things and not four aspects of one.
func howTo() string {
	var b strings.Builder
	b.WriteString(`<ol class="ib-howto">`)
	b.WriteString(`<li>Write to the agent address above from anywhere — your own ` +
		`mail, your phone. It answers on the same thread.</li>`)
	// Not "give it a job rather than a question".
	//
	// That said the agent picks work up while you are elsewhere and replies
	// when it is done, and nothing on this page does that. Mail arriving runs
	// exactly one turn — service/mail publishes, agent/mail calls agent.Ask,
	// the answer goes back on the thread — and there is no queue behind it, no
	// resumption and nothing that outlives the reply. Writing "do this over the
	// next hour" to the address gets an answer immediately, about the request.
	//
	// Work that outlives a message is real, and it is somewhere else:
	// service/tasks and service/events publish event.WorkForAgent, agent/work
	// runs it, and the answer returns to the thread it was asked on. So the
	// line now points at the thing that does it rather than promising it here.
	b.WriteString(`<li>For work that should happen later or on a schedule, make a ` +
		app.TextLink("task", "/tasks") +
		` — it runs while you are elsewhere and the answer arrives on this thread.</li>`)
	b.WriteString(`<li>Cc an agent into a conversation with somebody else and it ` +
		`follows along without taking it over.</li>`)
	b.WriteString(`<li>Or connect a mail client over ` +
		app.TextLink("imap", "/inbox/imap") +
		` and read all of it where you already read mail.</li>`)
	b.WriteString(`</ol>`)
	return b.String()
}

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
