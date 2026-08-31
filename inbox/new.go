package inbox

// Writing one yourself.
//
// The inbox could read and it could answer, and it could not start anything.
// That is a strange mailbox: half of what anybody does in one is write the
// first message.
//
// # It does not write it for you
//
// There was a second button here. **Draft** ran the agent over an instruction
// and filled the boxes; **Send** put it in the post. The argument was that the
// moment worth having an agent for is the one where the page is blank.
//
// It is gone, and the reason is what this page is for. The inbox is triage —
// read what came in, say something, hand work to an agent, send mail out. Every
// one of those is a decision. Writing the words is not the part anybody was
// stuck on, and a text box asking what to say, above four suggestion chips,
// made a page about deciding look like a page about composition. It also cost a
// credit and a model call to produce a paragraph you then edited.
//
// Handing work to an agent is still here and is the thing that was actually
// wanted: it is act.go, on a conversation, where there is something to act on.
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
// # Where the sent message goes
//
// Into internal/thread, keyed on the Message-ID it went out with. That is the
// same key agent/mail derives from a reply's In-Reply-To, so when they write
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
	"mu/internal/thread"
	"mu/service/mail"
	"mu/service/sms"
)

// bodyLimit bounds one message. A mail nobody would read is not a mail this
// form needs to be able to send.
const bodyLimit = 40000

// NewHandler serves /inbox/new.
//
// Not a constructor, despite the shape. A handler is named for the page it
// serves — see the naming rules — and the page is /inbox/new, which is what a
// mail client calls the thing this does. It was Compose, which is the verb for
// the half that has gone: there is no drafting here any more, only a blank
// message and a Send.
func NewHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	f := form{
		To:      strings.TrimSpace(r.FormValue("to")),
		Subject: strings.TrimSpace(r.FormValue("subject")),
		Body:    r.FormValue("body"),
		On:      strings.TrimSpace(r.FormValue("on")),
	}
	// A conversation somebody else's id names is not a conversation. Checked on
	// the way in rather than at the point it is written, so a forged id is a
	// blank form and never a message filed onto a stranger's thread.
	if f.On != "" && thread.Get(acc.ID, f.On) == nil {
		f.On = ""
	}
	if len(f.Body) > bodyLimit {
		f.Body = f.Body[:bodyLimit]
	}

	if r.Method != http.MethodPost {
		writeOne(w, r, acc.ID, f)
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "that request did not carry a valid token")
		return
	}

	sent(w, r, acc.ID, f)
}

// form is what is in the boxes, carried back when a send is refused.
//
// A struct rather than four arguments because every one of them survives that:
// a form that empties itself on a bad address is a form that eats your work.
type form struct {
	To      string
	Subject string
	Body    string
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

// sent puts it in the post and writes it down.
func sent(w http.ResponseWriter, r *http.Request, accountID string, f form) {
	switch {
	case f.To == "":
		f.Problem = "who is it to?"
		writeOne(w, r, accountID, f)
		return
	case strings.TrimSpace(f.Body) == "":
		f.Problem = "there is nothing in it yet"
		writeOne(w, r, accountID, f)
		return
	}

	// A person, resolved to an address at the last moment.
	//
	// The To box may hold @someone, which is what the Write button on a profile
	// puts there. That button used to carry the address itself — printed on the
	// page beside it and again in the link — and /@somebody is public, so every
	// account's mailbox was published to anybody who opened it or anything that
	// crawled it. The shortcut is worth keeping; publishing the address is not.
	//
	// Resolved here rather than when the form is drawn, so the address never
	// reaches the sender's browser at all. An @name nobody here answers to is
	// refused by name — an unhelpful "no such address" would be about a string
	// the sender never typed.
	if strings.HasPrefix(f.To, "@") {
		to, ok := addressOfPerson(f.To)
		if !ok {
			f.Problem = "nobody here is called " + f.To
			writeOne(w, r, accountID, f)
			return
		}
		f.To = to
	}

	// Deliver is the one way a message goes anywhere, and every rule about who
	// may send what is inside it — the gate, the charge, the provider, and the
	// branch on where the recipient actually is. A second path here that
	// skipped one of them would not look like a bug until the damage was done.
	// See service/mail/deliver.go.
	//
	// This called ReplyOut, which is the half of it for mail *leaving* — so
	// writing to somebody on this instance, or to your own agent, was refused
	// with "that is on this instance, that is not mail leaving it". Every other
	// door routed; the one a person types into did not.
	name := ""
	if acc, err := auth.GetAccount(accountID); err == nil {
		name = acc.Name
	}
	// The chain, when this is a reply. Without it the answer arrives at the
	// other end as a brand new conversation sitting next to the thread it
	// answers — which is what /inbox did: it called SendOut with an empty
	// replyTo and there was no references argument to pass anyway.
	// Delivered by whichever door the conversation is on, not by whichever one
	// this page was built for.
	//
	// One compose screen, one entry point, and the record decides the
	// transport. The alternative was a second form somewhere else that also
	// sends things, which is how a service ends up with two send paths and only
	// one of them counting the daily cap.
	if th := replyTarget(accountID, f); th != nil && onAPhone(th.Client) {
		sendText(w, r, accountID, f)
		return
	}

	inReplyTo, references := threadChain(accountID, f.On)
	f.Subject = replySubject(f.Subject, inReplyTo)
	messageID, err := mail.Deliver(mail.Outgoing{
		FromID: accountID, Display: name, To: f.To,
		Subject: f.Subject, Body: f.Body,
		InReplyTo: inReplyTo, References: references,
	})
	if err != nil {
		f.Problem = err.Error()
		writeOne(w, r, accountID, f)
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

// addressOfPerson turns @someone into the address their mail arrives at.
//
// Only for accounts on this instance, which is the only kind of name an @
// could refer to. It is deliberately not exposed anywhere a page can call it:
// the point of naming people rather than addressing them is that the address
// stays on the server.
func addressOfPerson(at string) (string, bool) {
	id := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(at, "@")))
	if id == "" {
		return "", false
	}
	acc, err := auth.GetAccount(id)
	if err != nil || acc == nil {
		return "", false
	}
	return mail.EmailForUser(acc.ID, mail.ConfiguredDomain()), true
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
	// The body alone. The subject went in front of it, so a conversation showed
	// its subject as the heading and again at the top of every message — see
	// thread.Name, which is how a conversation is named without writing the name
	// into what somebody said.
	thread.Name(accountID, th.ID, strings.TrimPrefix(f.Subject, "Re: "))
	// No From, so it is the owner speaking and is read the moment it is
	// written — see thread.Add. Ref is the Message-ID, which is also what stops
	// a bounce or a copy of this arriving back and being recorded twice.
	thread.Add(thread.Message{Thread: th.ID, Account: accountID, Text: f.Body, Ref: messageID})
	thread.Join(accountID, th.ID, thread.Party{Kind: thread.RolePerson, Key: f.To})
}

// threadChain is what a reply has to name to be threaded by the other side:
// the Message-ID of the message being answered, and every id the conversation
// has carried, oldest first.
//
// It comes off thread.Message.Ref, which is where the mail client records the
// Message-ID of anything that arrived or was sent. A conversation with no mail
// in it — a chat, a WhatsApp exchange — has no refs, and both come back empty,
// which is the right answer: there is no chain to join.
//
// The parent is the newest message that has an id — not the newest message,
// which may be the agent's own answer recorded without one, and not the oldest,
// which would thread the whole conversation under its opening message and lose
// the ordering a client draws from the chain.
//
// thread.Messages is oldest first, and a limit takes the newest N of them in
// that order — so the slice is already the order a References header wants, and
// the parent is the last id in it.
func threadChain(accountID, threadID string) (inReplyTo, references string) {
	if threadID == "" {
		return "", ""
	}
	var refs []string
	for _, m := range thread.Messages(accountID, threadID, chainDepth) {
		if m.Ref != "" {
			refs = append(refs, m.Ref)
		}
	}
	if len(refs) == 0 {
		return "", ""
	}
	return refs[len(refs)-1], strings.Join(refs, " ")
}

// chainDepth bounds how far back a References header reaches. Long enough for
// any conversation somebody is actually reading; short enough that a header
// does not grow without limit on a thread that has run for months — which is
// the failure mode References has, and why clients truncate it too.
const chainDepth = 40

// replySubject keeps a thread's subject recognisable.
//
// A client threads on the headers, not the subject — but a person scanning a
// mailbox reads the subject, and an answer titled the same as the question
// reads as a separate message from the same person. One "Re: ", never stacked:
// "Re: Re: Re:" is what happens when each end adds its own.
func replySubject(subject, inReplyTo string) string {
	subject = strings.TrimSpace(subject)
	if inReplyTo == "" || subject == "" {
		return subject
	}
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

// replyTarget is the conversation a reply belongs on, or nil for a new message.
//
// Re-checked here rather than trusted from the form, because this is the call
// that writes: NewHandler cleared a bad id on the way in, and a second look
// costs a map lookup and means the write cannot be reached with an id that was
// never checked.
func replyTarget(accountID string, f form) *thread.Thread {
	if f.On == "" {
		return nil
	}
	return thread.Get(accountID, f.On)
}

// mailClient is what the record calls a mail conversation. The same string
// agent/mail uses, so a sent message and the reply to it are one conversation
// on one client rather than two — it is a constant here rather than an import
// because agent/mail consumes tools and this package does not import those.
const mailClient = "mail"

// writeOne renders the form.
func writeOne(w http.ResponseWriter, r *http.Request, accountID string, f form) {
	var b strings.Builder
	b.WriteString(`<div class="ib">`)
	// Back where you came from. A reply reached from a conversation that offers
	// "← Inbox" sends you to the list, which is one step past where you were.
	back := app.TextLink("← Inbox", "/inbox")
	if f.On != "" {
		back = app.TextLink("← Back to the conversation", "/inbox?id="+url.QueryEscape(f.On))
	}
	b.WriteString(app.Actions(back))

	// A text has no subject and no address on either end. The same screen, told
	// what it is writing by the conversation it is answering — rather than a
	// second compose page that also sends things.
	texting, channel := false, sms.ChannelSMS
	if th := replyTarget(accountID, f); th != nil && onAPhone(th.Client) {
		texting = true
		if th.Client == thread.WhatsAppClient {
			channel = sms.ChannelWhatsApp
		}
	}
	if texting {
		from := sms.From()
		if channel == sms.ChannelWhatsApp {
			if senders := sms.SendersFor(channel); len(senders) > 0 {
				from = senders[0]
			}
		}
		b.WriteString(`<p class="ib-from-line">` + html.EscapeString(channel.Label()) +
			` to <code>` + html.EscapeString(f.To) + `</code> from <code>` +
			html.EscapeString(from) + `</code></p>`)
	}

	// Names inside, addresses outside.
	//
	// This said "From asim@micro.mu" whoever the message was for, including
	// somebody two rows away in the same directory. Within one instance the
	// domain is a routing fact and not part of who anybody is — it is the same
	// on both ends, so printing it says nothing and reads like posting a letter
	// to your flatmate. It is the identity the moment the message leaves, and
	// then it is shown.
	if texting {
		// Said above, with the number it goes to.
	} else if _, local := addressOfPerson(f.To); local && f.To != "" {
		b.WriteString(`<p class="ib-from-line">From <code>` +
			html.EscapeString(accountID) + `</code></p>`)
	} else if from := mail.EmailForUser(accountID, mail.ConfiguredDomain()); from != "" {
		b.WriteString(`<p class="ib-from-line">From <code>` + html.EscapeString(from) + `</code></p>`)
	}
	if f.On != "" && !texting {
		b.WriteString(`<p class="ib-from-line">Replying to <code>` +
			html.EscapeString(f.To) + `</code> — this lands on the same conversation.</p>`)
	}
	if f.Problem != "" {
		b.WriteString(`<p class="ib-ask-problem">` + html.EscapeString(f.Problem) + `</p>`)
	}

	b.WriteString(`<form class="ib-new" method="post" action="/inbox/new">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">`)
	if f.On != "" {
		b.WriteString(`<input type="hidden" name="on" value="` + html.EscapeString(f.On) + `">`)
	}
	// text, not email. The field takes a handle as readily as an address —
	// addressOfPerson resolves one — and type=email calls a handle invalid,
	// which it is, right up until the moment it is the thing you meant to type.
	// It was prefilled with "@asim" from the directory, so the browser was
	// being handed a value it would refuse.
	//
	// And shown without the @, because inside one instance a name is a name.
	if texting {
		// The number is the conversation. Editable would mean this one screen
		// could start a text to anybody, which is sms_send's job and is priced
		// and capped there.
		b.WriteString(`<input type="hidden" name="to" value="` + html.EscapeString(f.To) + `">`)
	} else {
		b.WriteString(`<input class="ib-field" type="text" name="to" required placeholder="To" value="` +
			html.EscapeString(strings.TrimPrefix(f.To, "@")) + `">`)
		b.WriteString(`<input class="ib-field" type="text" name="subject" placeholder="Subject" value="` +
			html.EscapeString(f.Subject) + `">`)
	}
	rows, placeholder, verb := "12", "Write it", "Send"
	if texting {
		// Three rows, because a text is short and a twelve-row box invites one
		// that is not — it is charged per segment and refused past three.
		rows, placeholder, verb = "3", "Keep it short — this is charged per segment", "Send text"
	}
	b.WriteString(`<textarea class="ib-field" name="body" rows="` + rows +
		`" placeholder="` + placeholder + `">` + html.EscapeString(f.Body) + `</textarea>`)

	b.WriteString(`<div class="ib-ask-row"><button type="submit">` + verb + `</button>`)
	b.WriteString(`</form></div>`)

	app.Respond(w, r, app.Response{Title: "New message", Description: "Write one",
		HTML: b.String()})
}

// sendText answers an SMS conversation with a text.
//
// Through sms.Send, which is where every rule about what a text costs lives —
// the daily cap, the country list, STOP, the charge, the segment limit. A
// second path that skipped any of them would not look like a bug until the
// bill arrived.
func sendText(w http.ResponseWriter, r *http.Request, accountID string, f form) {
	body := strings.TrimSpace(f.Body)
	if body == "" {
		f.Problem = "there is nothing to send"
		writeOne(w, r, accountID, f)
		return
	}
	// Back the way it came. A WhatsApp conversation answered by text arrives on
	// the other person's phone as a second thread from a number they do not
	// recognise, with nothing on it to say why — so the record decides the
	// channel here the same way it decides the transport above.
	channel := sms.ChannelSMS
	if th := replyTarget(accountID, f); th != nil && th.Client == thread.WhatsAppClient {
		channel = sms.ChannelWhatsApp
	}
	if _, err := sms.SendOn(channel, accountID, f.To, body); err != nil {
		f.Problem = err.Error()
		writeOne(w, r, accountID, f)
		return
	}
	// Recorded on the conversation it answers, so the thread reads as one
	// exchange rather than as a text that arrived and an answer that did not.
	//
	// thread.Add directly rather than record(), which names the conversation
	// from a subject and joins a mail address to it — a text has neither, and
	// the thread already has both its name and the number on it.
	if th := replyTarget(accountID, f); th != nil {
		thread.Add(thread.Message{Thread: th.ID, Account: accountID, Text: body})
	}
	http.Redirect(w, r, "/inbox?id="+url.QueryEscape(f.On), http.StatusSeeOther)
}
