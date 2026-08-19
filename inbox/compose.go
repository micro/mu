package inbox

// Writing one yourself, with the agent.
//
// The inbox could read and it could answer, and it could not start anything.
// That is a strange mailbox: half of what anybody does in one is write the first
// message. And it is a stranger agentic inbox, because the moment worth having
// the agent for is precisely the one where the page was blank — you know who to
// write to and roughly what to say, and the writing is the part you were putting
// off.
//
// So there are two buttons and they are not variations of each other. **Draft**
// runs the agent, costs a credit, and fills the box; **Send** puts it in the
// post. Nothing is ever sent by the agent on its own — the draft lands in a
// textarea you are looking at, and the send is a second, separate act by a
// person. An agent that could do both from one press is a different product with
// a different risk, and not one anybody asked for.
//
// # Why this imports the mail service
//
// Because sending mail needs it. inbox/doc.go says this package takes internal/
// and nothing that consumes tools, and service/mail is neither — it is a
// service, it provides tools rather than consuming them, and it imports nothing
// but internal/, so there is no cycle to break and no hook to justify.
// hooks.go's own rule applies: prefer a plain import, and take the hook only
// when you cannot have one. Here we can.
//
// The agent is the other half and stays a hook, because agent/ is the one import
// this package may not have. See Draft.
//
// # Where the sent message goes
//
// Into internal/thread, keyed on the Message-ID it went out with. That is the
// same key client/mail derives from a reply's In-Reply-To, so when they write
// back the answer lands on this conversation rather than starting a second one.
// Sending is how a thread begins; nothing else had to change for the reply to
// find it.

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/thread"
	"mu/service/mail"
)

// Draft asks the agent to write a message, and is filled in by the server
// because this package may not import agent/.
//
// It is handed what is already in the form — who it is to, and whatever has been
// typed — because "make it shorter" and "add the address" are instructions about
// a draft rather than requests for a new one. What comes back is the whole
// message: a subject on the first line, a blank line, then the body.
//
// Nil on a build with no agent, which hides the button rather than offering one
// that does nothing.
var Draft func(accountID, instruction, to, subject, body string) (string, error)

// draftLimit bounds one instruction to the agent, and bodyLimit one message.
// A mail nobody would read is not a mail this form needs to be able to send.
const (
	draftLimit = 2000
	bodyLimit  = 40000
)

// ComposeHandler serves /inbox/compose.
func ComposeHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	f := form{
		To:      strings.TrimSpace(r.FormValue("to")),
		Subject: strings.TrimSpace(r.FormValue("subject")),
		Body:    r.FormValue("body"),
		Ask:     strings.TrimSpace(r.FormValue("ask")),
		On:      strings.TrimSpace(r.FormValue("on")),
	}
	// A conversation somebody else's id names is not a conversation. Checked on
	// the way in rather than at the point it is written, so a forged id is a
	// blank compose form and never a message filed onto a stranger's thread.
	if f.On != "" && thread.Get(acc.ID, f.On) == nil {
		f.On = ""
	}
	if len(f.Body) > bodyLimit {
		f.Body = f.Body[:bodyLimit]
	}
	if len(f.Ask) > draftLimit {
		f.Ask = f.Ask[:draftLimit]
	}

	if r.Method != http.MethodPost {
		compose(w, r, acc.ID, f)
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "that request did not carry a valid token")
		return
	}

	// Which button. Draft rewrites the form; send empties it.
	if r.FormValue("draft") != "" {
		f = drafted(acc.ID, f)
		compose(w, r, acc.ID, f)
		return
	}
	sent(w, r, acc.ID, f)
}

// form is what is in the boxes, carried across a draft round trip.
//
// A struct rather than four arguments because every one of them survives a
// draft: an instruction that replaced the recipient you had typed would be a
// form that eats your work, which is the reason people stop pressing the button.
type form struct {
	To      string
	Subject string
	Body    string
	Ask     string
	Problem string
	Done    string
	// On is the conversation this answers, when it is one.
	//
	// A reply is a message with a thread already chosen, and that is the whole
	// difference from a new one. Without it a reply filed itself as its own
	// conversation and the page showed two: what they wrote, and what you sent
	// back, side by side, neither knowing about the other.
	On string
}

// drafted runs the agent over what is in the form and puts the answer back.
func drafted(accountID string, f form) form {
	switch {
	case Draft == nil:
		f.Problem = "there is no agent on this instance to write it"
		return f
	case f.Ask == "":
		f.Problem = "say what it should say, and the agent will write it"
		return f
	}
	// Charged like any other agent run, and before the model is asked so a run
	// that cannot be paid for does not spend one first.
	if ok, _, _, err := quota.CheckQuota(accountID, quota.OpAgentQuery); err != nil || !ok {
		f.Problem = "there are not enough credits for that"
		if err != nil {
			f.Problem = err.Error()
		}
		return f
	}

	out, err := Draft(accountID, f.Ask, f.To, f.Subject, f.Body)
	if err != nil {
		app.Log("inbox", "drafting a message failed: %v", err)
		f.Problem = "that one did not work. Try asking a different way."
		return f
	}
	quota.ConsumeQuota(accountID, quota.OpAgentQuery) //nolint:errcheck

	subject, body := split(out)
	if subject != "" {
		f.Subject = subject
	}
	if body != "" {
		f.Body = body
	}
	// The instruction has been carried out, so the box is empty for the next
	// one — which is nearly always "shorter" or "less formal", and typing that
	// after the first is what makes this a collaboration rather than a button.
	f.Ask = ""
	return f
}

// split reads a drafted message: subject on the first line, body after the
// blank one.
//
// The agent is told to answer in that shape. When it does not — and a model
// asked for two paragraphs will sometimes send three — the whole thing is the
// body and whatever subject was already typed stands. A wrong subject is worse
// than no subject, because it is the line the recipient reads first.
func split(out string) (subject, body string) {
	out = strings.TrimSpace(strings.ReplaceAll(out, "\r\n", "\n"))
	if out == "" {
		return "", ""
	}
	head, rest, found := strings.Cut(out, "\n\n")
	head = strings.TrimSpace(head)
	// A first line that is a paragraph is a paragraph. Subjects are short and
	// have no sentence in them; this is the same length a subject line is
	// truncated to everywhere else.
	if !found || head == "" || len(head) > 90 || strings.Contains(head, "\n") {
		return "", out
	}
	// "Subject: x" is what a model writes when told to put the subject first,
	// often enough that leaving it in would ship it to the recipient.
	head = strings.TrimSpace(strings.TrimPrefix(head, "Subject:"))
	return head, strings.TrimSpace(rest)
}

// sent puts it in the post and writes it down.
func sent(w http.ResponseWriter, r *http.Request, accountID string, f form) {
	switch {
	case f.To == "":
		f.Problem = "who is it to?"
		compose(w, r, accountID, f)
		return
	case strings.TrimSpace(f.Body) == "":
		f.Problem = "there is nothing in it yet"
		compose(w, r, accountID, f)
		return
	}

	// SendOut is the one way mail leaves this instance, and every rule about
	// who may send what is inside it — the gate, the charge, the provider. A
	// second path here that skipped one of them would not look like a bug until
	// the damage was done. See service/mail/outbound.go.
	name := ""
	if acc, err := auth.GetAccount(accountID); err == nil {
		name = acc.Name
	}
	messageID, err := mail.SendOut(accountID, name, f.To, f.Subject, f.Body, "", "")
	if err != nil {
		f.Problem = err.Error()
		compose(w, r, accountID, f)
		return
	}

	record(accountID, messageID, f)
	// Back to the conversation when there was one, because that is where the
	// answer now is. Sending from a reply and landing on the inbox list means
	// looking for the thread you were just reading.
	if f.On != "" {
		http.Redirect(w, r, "/inbox?id="+url.QueryEscape(f.On), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/inbox?sent="+url.QueryEscape(f.To), http.StatusSeeOther)
}

// record files what was sent as a conversation, keyed so the reply joins it.
func record(accountID, messageID string, f form) {
	th := replyTarget(accountID, f)
	if th == nil {
		key := messageID
		if key == "" {
			// Nothing to thread on, so it is at least its own conversation rather
			// than landing in somebody else's.
			key = "sent " + f.To + " " + f.Subject
		}
		th = thread.Open(accountID, mailClient, key)
	}
	if th == nil {
		return
	}
	text := f.Body
	if f.Subject != "" {
		text = f.Subject + "\n\n" + f.Body
	}
	// No From, so it is the owner speaking and is read the moment it is
	// written — see thread.Add. Ref is the Message-ID, which is also what stops
	// a bounce or a copy of this arriving back and being recorded twice.
	thread.Add(thread.Message{Thread: th.ID, Account: accountID, Text: text, Ref: messageID})
	thread.Join(accountID, th.ID, thread.Party{Kind: thread.RolePerson, Key: f.To})
}

// replyTarget is the conversation a reply belongs on, or nil for a new message.
//
// Re-checked here rather than trusted from the form, because this is the call
// that writes: ComposeHandler cleared a bad id on the way in, and a second look
// costs a map lookup and means the write cannot be reached with an id that was
// never checked.
func replyTarget(accountID string, f form) *thread.Thread {
	if f.On == "" {
		return nil
	}
	return thread.Get(accountID, f.On)
}

// mailClient is what the record calls a mail conversation. The same string
// client/mail uses, so a sent message and the reply to it are one conversation
// on one client rather than two — it is a constant here rather than an import
// because client/mail consumes tools and this package does not import those.
const mailClient = "mail"

// compose renders the form.
func compose(w http.ResponseWriter, r *http.Request, accountID string, f form) {
	var b strings.Builder
	b.WriteString(`<div class="ib">`)
	// Back where you came from. A reply reached from a conversation that offers
	// "← Inbox" sends you to the list, which is one step past where you were.
	back := app.TextLink("← Inbox", "/inbox")
	if f.On != "" {
		back = app.TextLink("← Back to the conversation", "/inbox?id="+url.QueryEscape(f.On))
	}
	b.WriteString(app.Actions(back))

	if from := mail.EmailForUser(accountID, mail.ConfiguredDomain()); from != "" {
		b.WriteString(`<p class="ib-from-line">From <code>` + html.EscapeString(from) + `</code></p>`)
	}
	if f.On != "" {
		b.WriteString(`<p class="ib-from-line">Replying to <code>` +
			html.EscapeString(f.To) + `</code> — this lands on the same conversation.</p>`)
	}
	if f.Problem != "" {
		b.WriteString(`<p class="ib-ask-problem">` + html.EscapeString(f.Problem) + `</p>`)
	}

	b.WriteString(`<form class="ib-compose" method="post" action="/inbox/compose">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">`)
	if f.On != "" {
		b.WriteString(`<input type="hidden" name="on" value="` + html.EscapeString(f.On) + `">`)
	}
	b.WriteString(`<input class="ib-field" type="email" name="to" required placeholder="To" value="` +
		html.EscapeString(f.To) + `">`)
	b.WriteString(`<input class="ib-field" type="text" name="subject" placeholder="Subject" value="` +
		html.EscapeString(f.Subject) + `">`)
	b.WriteString(`<textarea class="ib-field" name="body" rows="12" placeholder="Write it, or ask ` +
		`the agent to below">` + html.EscapeString(f.Body) + `</textarea>`)

	// The agent's half, under the message rather than beside it: it is a way of
	// filling the box above, so it reads in the order it is used.
	if Draft != nil {
		b.WriteString(`<div class="ib-draft">`)
		b.WriteString(`<input class="ib-field" type="text" name="ask" maxlength="2000" value="` +
			html.EscapeString(f.Ask) + `" placeholder="Tell the agent what to write — it fills the boxes above">`)
		b.WriteString(`<div class="ib-ask-row"><button type="submit" name="draft" value="1" class="pill">Draft</button>`)
		for _, s := range []string{
			"Write a short, friendly note",
			"Make it shorter",
			"Make it more formal",
			"Add what I know about them",
		} {
			b.WriteString(`<button type="button" class="pill" onclick="this.form.ask.value='` +
				html.EscapeString(s) + `';this.form.ask.focus()">` + html.EscapeString(s) + `</button>`)
		}
		b.WriteString(`</div></div>`)
	}

	b.WriteString(`<div class="ib-ask-row"><button type="submit">Send</button>`)
	b.WriteString(`<span class="ib-ask-note">Nothing is sent until you press Send. ` +
		`The agent writes; you post it.</span></div>`)
	b.WriteString(`</form></div>`)

	app.Respond(w, r, app.Response{Title: "Compose", Description: "Write one, with the agent",
		HTML: b.String()})
}

// composeLink is the way in, on the inbox itself.
//
// Drawn only where mail can actually leave: an instance with no mail domain
// configured has nowhere to send from, and a Compose button that always ends in
// "there is no mail domain here" is worse than no button. MaySendOut is the
// other half and is deliberately not checked — it depends on the recipient, and
// refusing before somebody has typed one would be guessing.
func composeLink() string {
	if !mail.Reachable() {
		return ""
	}
	return `<a class="pill ib-compose-link" href="/inbox/compose">Compose</a>`
}
