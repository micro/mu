package mail

// The first thing in a new account's inbox is the agent saying hello.
//
// Signing up used to end on the home screen, in front of a grid of news and
// markets cards and a banner suggesting you go and connect something over MCP.
// Every one of those is a page to read. Nothing had happened *to* you, and the
// product's claim is that you hand work to an agent and it answers on the
// conversation you asked in — which is a claim best made by doing it.
//
// So it is a message, from the agent, in the inbox, before anything else. Not
// a page and not a modal: the point is that the inbox is where things turn up,
// and the way to teach that is for something to turn up in it. Reply and the
// agent answers, because the address it comes from is the one that wakes it —
// so the first interaction is the product working rather than a tour of it.
//
// # What it says
//
// Four things, and no more: your own address, that replying works, where the
// tools are, and that talking is free while paid tools need credit. Plus who
// to write to when something is wrong, which is the operator.
//
// The operator's address is on it deliberately, and it is not the same
// decision as the one taken on public profiles — where an account's address
// was being published to anybody who opened the page, and is not any more. This
// is one address, of the person who chose to run the instance, in a private
// message to somebody who just joined it. Somebody running a service is
// contactable; that is what running a service means.
//
// # Why not a template file
//
// Because it is four sentences that have to agree with the product — the
// address scheme, the price of talking, the name of the tools page — and a
// template in a separate file is where those quietly stop agreeing. It is also
// deliberately not written by the model: a welcome that varies per account
// cannot be checked, and the one thing it must do is be correct.

import (
	"fmt"
	"html"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/origin"
	"mu/service/mail"
)

// Welcome starts the greeter. Called at boot, beside Load.
func Welcome() {
	go func() {
		sub := event.Subscribe(event.AccountCreated)
		for e := range sub.Chan {
			id, _ := e.Data["account"].(string)
			name, _ := e.Data["name"].(string)
			if id == "" {
				continue
			}
			greet(id, name)
		}
	}()
}

// greet delivers the message. Local delivery, so it lands in the mailbox, in
// IMAP and — since every local delivery reaches the record — at /inbox.
//
// On every instance, including one with no mail domain set. service/mail says
// what that costs and it is only the outside world: "an inbox, a handle, an
// agent that answers, messages between accounts — works with nothing
// configured at all". The welcome is all four of those. What changes is what
// it can honestly say about addresses, which is welcomeBody's problem.
func greet(accountID, name string) {
	// A Message-ID of our own, so their reply threads onto this conversation
	// rather than starting a second one beside it. ConfiguredDomain here
	// rather than Domain, because this one is a protocol identifier and wants
	// the localhost fallback.
	id := fmt.Sprintf("<welcome.%d.%s@%s>",
		time.Now().UnixNano(), accountID, mail.ConfiguredDomain())

	if err := mail.DeliverHere(mail.Local{
		Display:   "Mu",
		From:      mail.SharedAgentAddress(),
		To:        accountID,
		Subject:   "Welcome — you can reply to this",
		Body:      welcomeBody(accountID, name),
		MessageID: id,
	}); err != nil {
		app.Log("welcome", "could not greet %s: %v", accountID, err)
	}
}

// welcomeBody is the message. HTML, because that is what the inbox renders.
//
// mail.Domain, not mail.ConfiguredDomain: the second answers "localhost" when
// nothing is set, so that the SMTP handshake has a hostname to greet with, and
// the package says in as many words that using it for anything a person reads
// is a bug it has already had — "an instance with no mail server told an agent
// its address was asim@localhost". Here that would be the first sentence of
// the first message somebody ever gets.
func welcomeBody(accountID, name string) string {
	domain := mail.Domain()
	who := strings.TrimSpace(name)
	if who == "" || strings.EqualFold(who, accountID) {
		who = ""
	} else {
		who = " " + firstName(who)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<p>Hello%s — I'm your agent.</p>", html.EscapeString(who))

	// The one instruction, first, because it is the only one that has to be
	// followed for any of the rest to matter.
	b.WriteString("<p><b>Reply to this message and I'll answer.</b> " +
		"That is the whole product: you write, I go and do it, the answer comes " +
		"back on this conversation. It works from here, from your mail client, " +
		"or from the chat on the home screen — same agent, same memory.</p>")

	// Their address. It is the thing they now have that they did not have an
	// hour ago, and nothing else on the site says it as plainly.
	//
	// Or their handle, on an instance nobody outside can write to. Same
	// sentence, one fewer promise — the tag still names an agent, it just
	// cannot be reached by mail from elsewhere.
	if domain != "" {
		fmt.Fprintf(&b, "<p>You have an address: <code>%s@%s</code>. "+
			"Anything sent there arrives in your inbox here. Add a tag — "+
			"<code>%s+notes@%s</code> — and it is one of your own agents, "+
			"which you can make at %s.</p>",
			html.EscapeString(accountID), html.EscapeString(domain),
			html.EscapeString(accountID), html.EscapeString(domain), link("/agents"))
	} else {
		fmt.Fprintf(&b, "<p>You are <code>%s</code> here. Add a tag — "+
			"<code>%s+notes</code> — and it is one of your own agents, which "+
			"you can make at %s.</p>",
			html.EscapeString(accountID), html.EscapeString(accountID), link("/agents"))
	}

	// What it can reach for.
	tools := ""
	if app.ToolCountFunc != nil {
		tools = fmt.Sprintf(" All %d of them, on this one server.", app.ToolCountFunc())
	}
	fmt.Fprintf(&b, "<p>I have tools: news, mail, search, weather, markets, "+
		"video, places, files, contacts, calendar, documents.%s They are listed "+
		"at %s, which is also where you connect your own agent — Claude, or "+
		"anything that speaks MCP — to the same set.</p>",
		tools, link("/tools"))

	// Money, said once and without ceremony.
	b.WriteString("<p>Talking to me is free. Tools that cost us something to run — " +
		"a model call, a paid third party — cost credits, and you can top up at " +
		link("/wallet/topup") + " when you need to. Reading and listing and " +
		"anything this instance stores itself stays free.</p>")

	// And a person, when the agent is not the answer.
	if op := operatorAddress(); op != "" {
		fmt.Fprintf(&b, "<p>If something is broken or you want to ask a person, "+
			"write to <code>%s</code> — that is whoever runs this instance.</p>",
			html.EscapeString(op))
	}

	return b.String()
}

// operatorAddress is where to write to a person, or nothing on an instance
// with nobody running it yet.
//
// Their address here, never the address they signed up with. The first is the
// instance's own and is the one they are contactable on as its operator; the
// second is their personal mail and is not ours to hand out.
func operatorAddress() string {
	op := auth.Operator()
	domain := mail.Domain()
	if op == "" || domain == "" {
		return ""
	}
	return mail.EmailForUser(op, domain)
}

// firstName is what somebody is called, for one greeting. "Asim Aslam" is how
// you address an envelope, not how you say hello.
func firstName(name string) string {
	if i := strings.IndexAny(name, " \t"); i > 0 {
		return name[:i]
	}
	return name
}

// link is an absolute link to a page here, with the address as its text.
//
// Absolute because this is mail: a relative href points at wherever the mail
// client thinks it is, which is nowhere. And the URL as the text, so that the
// plain-text version — which is what reaches internal/thread and so /inbox —
// still carries the address after the tags come off.
func link(path string) string {
	at := strings.TrimSuffix(origin.Self(), "/") + path
	return `<a href="` + html.EscapeString(at) + `">` + html.EscapeString(at) + `</a>`
}
