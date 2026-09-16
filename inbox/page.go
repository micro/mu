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

	"strconv"
	"strings"
	"time"

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
	if id := r.URL.Query().Get("id"); id != "" {
		kind := kindOf(r.URL.Query().Get("kind"))
		if kind == kindNote || kind == kindTask {
			itemPage(w, r, acc.ID, kind, id)
			return
		}
	}
	auth.SetCSRFCookie(w, r)
	if r.Method == http.MethodGet && app.WantsJSON(r) {
		clientData(w, r, acc.ID)
		return
	}
	// An instruction about the conversation being read. POST here rather than at
	// a path of its own, because /inbox/<box> is a mailbox name and /inbox/act
	// would be one an account could have.
	//
	// Searching posts here too, for the same reason and one more: what somebody
	// looks for in their own mail does not go in a URL, so the box posts. The two
	// are told apart by which field arrived — a search carries q, an instruction
	// carries ask or an action — rather than by a second route, because a second
	// route under /inbox is a mailbox name somebody could claim.
	if r.Method == http.MethodPost {
		if r.FormValue("action") == "handled" {
			if !auth.StrictCSRF(r) {
				app.Forbidden(w, r, "Invalid CSRF token")
				return
			}
			reviewed, err := time.Parse(time.RFC3339Nano, r.PostFormValue("reviewed"))
			if err != nil {
				app.BadRequest(w, r, "Missing message timestamp")
				return
			}
			thread.HandleAt(acc.ID, r.PostFormValue("id"), reviewed)
			http.Redirect(w, r, "/inbox", http.StatusSeeOther)
			return
		}
		// Parsed here so PostForm is populated: the two are told apart by whether
		// the field was sent at all, not by whether it has a value, because
		// pressing Search on an empty box is a search that found everything and
		// not an instruction with nothing in it.
		_ = r.ParseForm()
		if _, searching := r.PostForm["q"]; searching {
			if app.WantsJSON(r) {
				clientData(w, r, acc.ID)
			} else {
				priority(w, r, acc.ID)
			}
			return
		}
		action(w, r, acc.ID)
		return
	}
	if id := r.URL.Query().Get("id"); id != "" {
		conversation(w, r, acc.ID, id)
		return
	}
	priority(w, r, acc.ID)
}

// boxOf is which mailbox the path asks for, empty for all of them.
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

	var tags []string
	if name := agentLabel(accountID, t.Agent); name != "" {
		tags = append(tags, trimTo(name, labelChars))
	}

	// Unread, which is what makes this a mailbox rather than a log. Without it
	// every row looks the same and the page has to be read top to bottom every
	// time, because nothing says which of these you have dealt with.
	cls := "list-link ib-row"
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
	// meanings. The row reserves an action column even when there is no form.
	return `<div class="ib-item">` +
		`<a class="` + cls + `" href="` + html.EscapeString(inboxURL(r, t.ID)) + `"` + titleAttr(full) + `>` +
		rowMeta(who, app.ClientName(t.Client), tags, time.Time{}) +
		`<span class="ib-subject">` + html.EscapeString(trimTo(subject, 90)) + `</span>` +
		`<span class="ib-snip text-muted">` + html.EscapeString(snippet) + `</span></a>` +
		rowDelete(r, t.ID) + rowTime(t.Updated) + `</div>`
}

// rowMeta gives every inbox kind the same label and date positions. The
// context can wrap on narrow screens; the date keeps its own column.
func rowMeta(who, kind string, tags []string, at time.Time) string {
	context := ""
	if who != "" {
		context = `<span class="ib-who metadata-name">` + html.EscapeString(who) + `</span>`
	}
	if kind != "" {
		context += `<span class="ib-kind metadata-kind">` + html.EscapeString(kind) + `</span>`
	}
	if len(tags) > 0 {
		context += `<span class="ib-tags metadata-detail">` + html.EscapeString(strings.Join(tags, " · ")) + `</span>`
	}
	return `<span class="ib-meta metadata-row metadata-columns"><span class="metadata-identity">` + context +
		`</span>` + rowTime(at) + `</span>`
}

func rowTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return `<time class="ib-when metadata-time" datetime="` + at.Format(time.RFC3339) + `">` + html.EscapeString(app.TimeAgo(at)) + `</time>`
}

// rowDelete is the cross at the end of a row.
//
// A glyph rather than the word, because it repeats down the page and twenty
// "Delete"s is a column of warnings. It is labelled for anything not reading
// the shape, and it asks first — thread.Delete is not recoverable.
func rowDelete(r *http.Request, id string) string {
	return `<form class="form-action ib-del" method="post" action="/inbox/delete" ` +
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
func conversation(w http.ResponseWriter, r *http.Request, accountID, id string, draft ...form) {
	w.Header().Set("Cache-Control", "no-store")
	auth.SetCSRFCookie(w, r)
	t := thread.Get(accountID, id)
	if t == nil {
		// Scoped to the reader by thread.Get, so somebody else's id is not
		// "forbidden" — it is not a thing that exists here.
		app.NotFound(w, r, "no conversation here with that id")
		return
	}

	// Web conversations resume in the shared composer, preserving the owned thread.
	if t.Client == thread.WebClient {
		thread.MarkSeen(accountID, t.ID)
		http.Redirect(w, r, "/agent?session="+url.QueryEscape(t.ID), http.StatusSeeOther)
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
	b.WriteString(`<div class="ib page-stack">`)
	// Where you came from, and what you can do to this — one bar rather than
	// three loose things stacked above the conversation. See app.Actions.
	toolbar := []string{unreadButton(r, t.ID, wasUnread), deleteButton(r, t.ID)}

	all := inboxThreads(accountID, r.URL.Path)
	for i, item := range all {
		if item.ID != id {
			continue
		}
		var links []string
		if i > 0 {
			links = append(links, app.TextLink("Previous", inboxURL(r, all[i-1].ID)))
		}
		if i+1 < len(all) {
			links = append(links, app.TextLink("Next", inboxURL(r, all[i+1].ID)))
		}
		if len(links) > 0 {
			toolbar = append(links, toolbar...)
		}
		break
	}

	b.WriteString(app.Actions(app.TextLink("Inbox", inboxURL(r, "")), toolbar...))

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
		assignDialog(r, accountID, t, replyTo(accountID, t, msgs)), inlineReply(r, accountID, t, msgs, draft...)))
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
