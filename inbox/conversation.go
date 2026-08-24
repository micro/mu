package inbox

// Reading a conversation that did not happen here.
//
// There is one list of conversations and it is the rail on /agent. There was
// briefly a second page listing the same conversations under the heading
// Threads, beside a third listing the workflow records behind them under the
// heading Runs, and a tab strip switching between the three. Four names —
// chat, threads, runs, connect — for what is one thing: you, an agent, and
// what you have said to each other.
//
// So the rail lists everything, whichever client it happened on. A conversation
// from the web opens in the chat and can be continued. One that happened by
// email or on WhatsApp opens here instead: the same messages, read-only, with a
// line saying where it took place and how to carry it on — which is by replying
// there, not by typing into a box on this page that would send nothing.

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
)

// SessionHandler serves /agent/session/<id>: DELETE removes a conversation.
//
// The rail lists conversations, so the thing it deletes is a conversation. It
// used to DELETE a workflow record, because the rail was built out of those.
func SessionHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		app.MethodNotAllowed(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/agent/session/")
	if id == "" {
		app.NotFound(w, r, "no conversation named")
		return
	}
	thread.Delete(acc.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

// MessagesShown bounds one conversation.
//
// A mail thread somebody has been adding to for a year is the case this page
// exists for, and rendering every message of it — each through the markdown
// renderer — is a page that takes a second to draw and that nobody reads the
// top of. The most recent are the ones being read.
const MessagesShown = 100

// ConversationView renders a conversation from another client, read-only.
//
// Everything on it, in one column, which is what a reader who is not in their
// inbox wants — the chat page opens a mail thread this way and there is no
// agent panel beside it there to put the agent's turns in.
func ConversationView(accountID string, t *thread.Thread) string {
	msgs := thread.Messages(accountID, t.ID, MessagesShown)
	return conversationPane(accountID, t, msgs, len(msgs) >= MessagesShown, true, "")
}

// conversationPane is the correspondence: a heading, who is on it, and the
// messages given to it.
//
// It takes the messages rather than reading them, because the inbox shows it a
// subset. There, the agent's turns and the owner's instructions to it are the
// panel on the right and this column is what actually passed between people —
// see aside.
// titled says whether the pane draws the subject itself.
//
// It always did, and on its own page that is the subject twice: app.Respond
// renders the page's Title as the heading, and /inbox?id= passes the subject as
// the Title — so the conversation opened with its name, then the Inbox and
// Delete bar, then its name again. Embedded in somebody else's page it is the
// other way round: the heading is that page's, and without this the pane is a
// conversation with nothing saying which.
// assign is the Assign dialog, already rendered, or "" for a caller that is
// not offering one.
//
// Passed in rather than built here so the button and the dialog cannot come
// apart: actionBar draws the button only when there is a dialog to open, and
// the dialog is emitted by this function at the end. They were separate for
// one commit and that was enough — ConversationView drew the button and the
// inbox page drew the dialog, so opening a mail thread from /agent got a
// button that did nothing. Same shape as the agent page calling a function
// defined in a panel it had stopped rendering.
func conversationPane(accountID string, t *thread.Thread, msgs []thread.Message, trimmed, titled bool, assign string) string {
	subject := t.Subject
	if subject == "" {
		subject = "Untitled"
	}

	var b strings.Builder
	b.WriteString(`<div class="ib-conv"><div class="ib-head"><span class="pill">` +
		html.EscapeString(app.ClientName(t.Client)) + `</span><span class="ib-started">started ` +
		html.EscapeString(app.TimeAgo(t.Started)) + `</span></div>`)
	if titled {
		b.WriteString(`<h2 class="ib-title">` + html.EscapeString(subject) + `</h2>`)
	}
	b.WriteString(partyLine(accountID, t))
	if trimmed {
		b.WriteString(`<p class="ib-trimmed">Showing the most recent ` +
			strconv.Itoa(MessagesShown) + `. ` +
			app.Link("Search the whole conversation", "/recall") + `</p>`)
	}

	for _, m := range msgs {
		b.WriteString(messageBlock(accountID, t, m, subject))
	}

	// The two things you can do with a conversation, on one line.
	//
	// Replying is mail only: the rest are somebody else's transport — a room
	// thread is answered in the room — which is what the note underneath says.
	// It could not reply at all once, and said so, which described an inbox you
	// cannot answer from. Assign is offered wherever the caller has a dialog for
	// it to open.
	to := replyTo(accountID, t, msgs)
	b.WriteString(actionBar(t, to, assign != ""))
	if to == "" {
		b.WriteString(`<p class="ib-note">This happened on ` +
			html.EscapeString(app.ClientName(t.Client)) + `, so a reply carries on there — answer it ` +
			`the way it arrived and the agent picks it up in the same thread.</p>`)
	}
	b.WriteString(`</div>`)
	// Last, and outside .ib-conv: a <dialog> inside the conversation would sit
	// inside its column and inherit its width.
	b.WriteString(assign)
	return b.String()
}

// replyTo is the address a reply goes to, or empty where there is nobody to
// reply to.
//
// The most recent person who is not you, rather than the oldest or the party
// list, because a thread three people have written on is answered to whoever
// spoke last — and because the party list has no order in it, so it cannot tell
// you that.
//
// Only mail. A conversation from the web has an address on nothing.
func replyTo(accountID string, t *thread.Thread, msgs []thread.Message) string {
	if t.Client != mailClient {
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == thread.RoleAgent {
			continue
		}
		if from := strings.TrimSpace(m.From); from != "" && from != accountID {
			return from
		}
	}
	return ""
}

// A link rather than a box for Reply, because the New page is where a message
// is written and there is no reason for a second half-sized version of it here.
// It arrives with the recipient and the subject filled in and the conversation
// attached, so what comes back joins this thread instead of starting one.
//
// app.ActionLink, not a class of its own. The first version of this was
// hand-drawn — a black pill with color:#fff — and mu.css carries a global
// `a:visited { color: #000 }`, which outranks a plain class selector. So the
// button was legible until you used it and black-on-black afterwards. That is
// the exact bug `a.btn` already carries `color: #fff !important` to prevent:
// the site has one button and it has been fixed once. Drawing a second one
// re-earns every bug the first one has already had.
// actionBar is what you can do with the conversation you are reading: answer
// it yourself, or give it to an agent.
//
// One row, and Assign is on it rather than under the thread.
//
// The instruction box used to sit permanently below the conversation — a
// two-row textarea, three suggestion pills and a paragraph of caption, on every
// conversation whether or not you had any intention of handing it over. That
// is a lot of furniture for something you do occasionally, and it pushed the
// thing you came to read up the page. Reported as "clutters the view", and the
// deeper complaint was that assigning did not read as one of the things you do
// with a message: it read as a second, stranger reply box.
//
// So it is a button beside Reply, and the box it opens is a dialog. Same two
// verbs, same weight, one row.
func actionBar(t *thread.Thread, to string, canAssign bool) string {
	var b strings.Builder
	b.WriteString(`<div class="ib-reply">`)
	if to != "" {
		subject := strings.TrimSpace(t.Subject)
		if subject == "" {
			subject = "your message"
		}
		// One Re:, however many times a subject has been round.
		if !strings.HasPrefix(strings.ToLower(subject), "re:") {
			subject = "Re: " + subject
		}
		q := url.Values{"to": {to}, "subject": {subject}, "on": {t.ID}}
		b.WriteString(app.ActionLink("/inbox/new?"+q.Encode(), "Reply"))
	}
	// The button only opens the dialog, so it carries no state and needs no
	// form — and it is drawn only where there is a dialog to open. See
	// conversationPane's assign parameter.
	if canAssign {
		b.WriteString(`<button type="button" class="ib-assign-open" ` +
			`onclick="muAssignOpen()">Assign to agent</button>`)
	}
	if to != "" {
		b.WriteString(`<span class="ib-reply-who">Reply goes to ` +
			html.EscapeString(to) + `</span>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// partyLine says who is on a conversation.
//
// Worth its own line because a thread is not always two-sided, and where it is
// not, nothing on the page said so: a stranger writes to agent@, the agent
// answers, and the owner reads an exchange they were never in. "You" and
// "Agent" describe that badly enough to be misleading.
//
// Silent for the ordinary case — you and your agent, which every conversation
// started here is — because a line naming the two people already obvious from
// the messages is furniture.
func partyLine(accountID string, t *thread.Thread) string {
	people := 0
	var names []string
	for _, p := range thread.Parties(accountID, t.ID) {
		if p.Kind == thread.RoleAgent {
			continue
		}
		people++
		names = append(names, partyName(p))
	}
	if people < 2 {
		return ""
	}
	return `<div class="ib-parties">Between ` + html.EscapeString(strings.Join(names, ", ")) +
		` and the agent</div>`
}

// withoutSubject drops a leading line that is only the conversation's subject.
//
// Mail used to be recorded as "Subject\n\nbody", because that was the only way
// a thread learned what it was about — see thread.Name, which is how a client
// says so now. Messages written before that are still on disk with the subject
// inside them, and a reader of an old conversation should not see it once as
// the heading and again on every message under it.
//
// Only an exact match, with or without a reply marker, and only as the first
// line. A message that happens to begin with the same words as its subject is
// somebody writing that, and cutting it would be deleting what they said.
func withoutSubject(text, subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return text
	}
	first, rest, ok := strings.Cut(text, "\n")
	if !ok {
		return text
	}
	head := strings.TrimSpace(first)
	for _, prefix := range []string{"re:", "fwd:", "fw:"} {
		if strings.HasPrefix(strings.ToLower(head), prefix) {
			head = strings.TrimSpace(head[len(prefix):])
		}
	}
	if !strings.EqualFold(head, subject) {
		return text
	}
	return strings.TrimLeft(rest, "\n")
}

// partyName is what to call somebody: the name the client knew, the address
// they wrote from, or "You" for the account this conversation belongs to.
func partyName(p thread.Party) string {
	switch {
	case p.Name != "":
		return p.Name
	case p.Key != "":
		return p.Key
	}
	return "You"
}

// messageBlock is one message. What a person wrote is escaped and shown as
// typed; what an agent wrote is markdown, rendered the way the chat renders it —
// through the untrusted renderer, because model output is exactly what that
// renderer is for.
func messageBlock(accountID string, t *thread.Thread, m thread.Message, subject string) string {
	m.Text = withoutSubject(m.Text, subject)
	if m.Role == thread.RoleAgent {
		ran := ""
		if m.Workflow != "" {
			ran = runTools(m.Workflow)
		}
		return `<div class="ib-msg ib-agent">` + fromLine("Agent", m.At) +
			`<div class="ib-body">` + app.RenderString(m.Text) + `</div>` + ran + `</div>`
	}
	// The author, by the name the conversation knows them under rather than the
	// address on the message. A thread where three people have written is three
	// names; one where the display name arrived later says it on every line.
	who := "You"
	if m.From != "" {
		who = m.From
		for _, p := range thread.Parties(accountID, t.ID) {
			if p.Kind == thread.RolePerson && p.Key == m.From && p.Name != "" {
				who = p.Name
				break
			}
		}
	}
	// What they wrote, and — folded away — the part of it that is this
	// conversation quoted back at itself. See quoted.go.
	body, quote := unquoted(m.Text)
	// Escaped, then linked. The agent's own answers go through the markdown
	// renderer above and come out with working links, and what a person wrote
	// came out as text — so the same URL in the same conversation was clickable
	// on one line and not on the next. See app.Linkify for the ordering.
	return `<div class="ib-msg ib-person">` + fromLine(who, m.At) + addressLine(m) +
		`<div class="ib-body ib-typed">` + app.Linkify(html.EscapeString(body)) + `</div>` +
		quotedBlock(quote) + `</div>`
}

// quotedBlock is the fold: a control that says there is more and shows it.
//
// A <details>, so it needs no script and no state kept anywhere — the browser
// already has this element and every reader already knows the shape from the
// same three dots in Gmail. Closed by default, because the thing behind it is
// the messages above.
func quotedBlock(quoted string) string {
	if strings.TrimSpace(quoted) == "" {
		return ""
	}
	return `<details class="ib-quoted"><summary title="Show quoted text" ` +
		`aria-label="Show quoted text">&middot;&middot;&middot;</summary>` +
		`<div class="ib-quoted-text">` + app.Linkify(html.EscapeString(quoted)) + `</div></details>`
}

// fromLine is who wrote a message and when, on one line with the time pushed to
// the right.
//
// It was "Henrik · 2 hours ago" in one muted grey run, which reads as a single
// caption rather than as two facts — and the name, which is what you scan a
// thread for, was the same weight as everything around it. The name is the
// text colour and the time is muted at the far end, the way a mail client sets
// it, so a thread reads down its left edge.
func fromLine(who string, at time.Time) string {
	return `<div class="ib-from"><span class="ib-who-l">` + html.EscapeString(who) +
		`</span><span class="ib-at">` + html.EscapeString(app.TimeAgo(at)) + `</span></div>`
}

// Tools renders which tools produced an answer, and is filled in by the agent
// because the workflow record is the agent's own — how an answer was made is a
// different question, with a different lifetime, from what was said.
//
// Nil on a build with no agent wired in, and silent when the run has expired,
// which it will: workflow records are evicted and messages are not, so an old
// conversation keeps what was said and loses how it was produced. That
// asymmetry is intended.
var Tools func(workflow string) string

func runTools(workflow string) string {
	if Tools == nil {
		return ""
	}
	return Tools(workflow)
}

// addressLine is who a message was from and who it was sent to, in full.
//
// The addresses, not the names — the name is on the line above and this is the
// line you read when you want to know exactly who wrote and which of your
// addresses they wrote to. A conversation could be read end to end without ever
// saying either: the sender was truncated into a column and the recipient was
// never recorded at all.
//
// Which address it arrived at is the fact that explains why the message is in
// this inbox. you@ is you; you+research@ is one of your agents; agent@ is this
// instance's. Same message, three different reasons for it to be here.
//
// Empty for anything with neither, which is every message you wrote yourself —
// "From: you, To: nobody" is furniture on your own words.
func addressLine(m thread.Message) string {
	from := strings.TrimSpace(m.From)
	to := strings.TrimSpace(m.To)
	if from == "" && to == "" {
		return ""
	}
	var parts []string
	if from != "" {
		parts = append(parts, `<span class="ib-addr-k">from</span> `+html.EscapeString(from))
	}
	if to != "" {
		parts = append(parts, `<span class="ib-addr-k">to</span> `+html.EscapeString(to))
	}
	return `<div class="ib-addrs">` + strings.Join(parts, `<span class="ib-addr-sep">·</span>`) + `</div>`
}
