package inbox

// Reading this inbox in the mail client you already have.
//
// The pitch is agents with addresses, and an address is worth something in
// proportion to how ordinary it is to reach. service/mail speaks IMAP for
// exactly that reason — see its package comment — and until this page existed
// there was nowhere that said so: the capability shipped, and the four facts a
// client asks for (host, port, username, password) were in a docs page about
// installing the server.
//
// So this is a settings card rather than an explanation. What a mail client
// wants, in the order its form asks for it, with the parts this instance
// actually knows filled in and the parts it cannot know said plainly.
//
// # What it cannot know
//
// Which port a reader should type. Nothing in this repo terminates TLS — the
// IMAP listener runs in the clear behind a proxy that does — so the port the
// process is bound to is usually not the port a client connects to. An operator
// who followed docs/INSTALL.md has nginx on 993 in front of 1143, and a page
// that reported 1143 would be telling everybody the wrong number.
//
// IMAP_PUBLIC is the operator's answer: what to tell people. Unset, the page
// says 993 with TLS when the listener is on a development port, because that is
// what the install guide produces and what every client offers first — and says
// the local port too, so somebody running without a proxy is not stuck.

import (
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
	"mu/service/mail"
)

// ImapHandler serves /inbox/imap.
//
// Signed in, because every fact on it is about one account — the username is
// the account, and the password is a token minted against it. Signed out there
// is nothing to render but the protocol's name.
func ImapHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	var b strings.Builder
	b.WriteString(`<div class="ib-imap">`)
	// Your inbox, and it means the inbox.
	//
	// This said "Everything that arrives here shows up there" when it served
	// the mail store alone, which was not true and not nearly true. It was
	// corrected to say mail only — and that was the wrong repair, made on the
	// reasoning that IMAP is mail's protocol the way XMPP is chat's.
	//
	// It is not. SMTP delivers and XMPP delivers; IMAP delivers nothing at all.
	// It reads a message store and has no opinion about how anything got there,
	// which makes it a consumption protocol rather than a transport — so the
	// store worth pointing it at is the record every channel writes to. See
	// inbox/imapbridge.go. The sentence is true again, and this time the server
	// is what made it true.
	b.WriteString(`<p class="svc-lead">Every thread in one place, in the mail client you ` +
		`already use. Mail, texts, WhatsApp and chats all arrive in it, the agent's replies ` +
		`land in the thread, and each of your agents is a folder.</p>`)
	b.WriteString(`<p class="ib-imap-note">Conversations that are not mail show a sender ` +
		`built from where they came from — a text from <code>+447700900123</code> reads as ` +
		`<code>447700900123.sms@…</code> — so a client has an address to show, to thread ` +
		`on, and to reply to. Answering one goes back out the way it came in.</p>`)
	b.WriteString(`<p class="ib-imap-note">Those addresses answer a conversation and cannot ` +
		`start one. They are composed from the parts, so anybody who has seen one could ` +
		`write another — if the address were enough to send, knowing the pattern would be ` +
		`permission to text any number in the world from this instance's number.</p>`)

	host, port, secure, on := imapReach()
	if !on {
		b.WriteString(app.Problem("This instance is not serving IMAP. An admin turns it " +
			"on with IMAP_PORT; it is on by default, so this means it was set to off."))
		b.WriteString(`</div>`)
		app.Respond(w, r, app.Response{Title: "IMAP", Description: "Read your inbox in a mail client", HTML: b.String()})
		return
	}

	b.WriteString(`<table class="ib-imap-table"><tbody>`)
	b.WriteString(imapRow("Server", host))
	b.WriteString(imapRow("Port", port))
	b.WriteString(imapRow("Security", secure))
	b.WriteString(imapRow("Username", acc.ID))
	b.WriteString(`<tr><th>Password</th><td>An access token — ` +
		app.TextLink("mint one at /token", "/token") + `. Not your sign-in: there is ` +
		`no password on this account, and a token is revoked on its own without ` +
		`touching how you sign in.</td></tr>`)
	b.WriteString(`</tbody></table>`)

	// Sending, because a client that can only read is a client that cannot
	// reply — and the reply is the whole point of reading it there.
	if addr, sport, ssecure, son := submissionReach(); son {
		b.WriteString(`<h3 class="lead-15">Sending</h3>`)
		b.WriteString(`<p class="ib-imap-note">So the client can reply as you. Same ` +
			`username, same token.</p>`)
		b.WriteString(`<table class="ib-imap-table"><tbody>`)
		b.WriteString(imapRow("Server", addr))
		b.WriteString(imapRow("Port", sport))
		b.WriteString(imapRow("Security", ssecure))
		b.WriteString(`</tbody></table>`)
	}

	b.WriteString(`<h3 class="lead-15">What you can do there</h3>`)
	b.WriteString(`<ul class="ib-imap-list">`)
	b.WriteString(`<li>Read, reply, mark read, delete. A reply to the agent runs it, ` +
		`the same as replying on this page — and a reply to a text goes out as a text.</li>`)
	b.WriteString(`<li>Each agent is a folder — mail to <code>you+research@</code> ` +
		`is the Research folder.</li>`)
	b.WriteString(`<li>New mail shows up while the client sits open, within about ` +
		`twenty seconds. A text or a chat appears on the next refresh.</li>`)
	// Said rather than left to be discovered. A client that offers a verb the
	// server refuses looks broken; a reader told why does not go looking.
	b.WriteString(`<li>You cannot make, rename or delete folders. They are your ` +
		`agents and your spam, both worked out from what has arrived, so there is ` +
		`nothing for those to change.</li>`)
	b.WriteString(`</ul>`)

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "IMAP", Description: "Read your inbox in a mail client", HTML: b.String()})
}

// imapRow is one line of a client's settings form.
func imapRow(k, v string) string {
	return `<tr><th>` + html.EscapeString(k) + `</th><td><code>` +
		html.EscapeString(v) + `</code></td></tr>`
}

// imapReach is what to type into a mail client: host, port, security, and
// whether this instance serves IMAP at all.
func imapReach() (host, port, secure string, on bool) {
	addr, listening := app.ListenAddr("IMAP_PORT", app.IMAPPort)
	if !listening {
		return "", "", "", false
	}
	host, port, secure = advertised(settings.Get("IMAP_PUBLIC"), addr, "993")
	return host, port, secure, true
}

// submissionReach is the same question for outgoing mail.
func submissionReach() (host, port, secure string, on bool) {
	addr, listening := app.ListenAddr("SUBMISSION_PORT", app.SubmissionPort)
	if !listening {
		return "", "", "", false
	}
	host, port, secure = advertised(settings.Get("SUBMISSION_PUBLIC"), addr, "465")
	return host, port, secure, true
}

// advertised turns a listener address into what to tell somebody.
//
// The operator's own answer first — public, the value of IMAP_PUBLIC or
// SUBMISSION_PUBLIC. Failing that, the bound port when it is a port the outside
// world would use, and the standard TLS port when it is not: a listener on 1143
// is a development default or a thing behind a proxy, and in neither case is
// 1143 the number to print.
//
// The value rather than the key, because the caller reads its own setting by
// its literal name. docs/config_test.go scans source for a settings lookup with
// a literal key, to check every setting is documented and every documented
// setting is read, and a key passed as an argument is invisible to it — which is a fair scan: a setting
// nothing names in source is a setting nobody can find by grep either.
func advertised(public, addr, standard string) (host, port, secure string) {
	host = mail.ConfiguredDomain()
	if host == "" {
		host = "this instance"
	}

	if set := strings.TrimSpace(public); set != "" {
		h, p := splitHostPort(set)
		if h != "" {
			host = h
		}
		if p == "" {
			p = standard
		}
		return host, p, security(p)
	}

	_, bound := splitHostPort(addr)
	if n, err := strconv.Atoi(bound); err == nil && n > 0 && n < 1024 {
		// A privileged port is one somebody deliberately bound, and is the one
		// a client should be given.
		return host, bound, security(bound)
	}
	// Behind a terminator, or about to be. Both numbers, because a self-hoster
	// with no proxy needs the second and would otherwise be told a port that is
	// not open.
	return host, standard + " (or " + bound + " with no TLS in front)", "SSL/TLS"
}

// security is what a client's dropdown should say for a port.
func security(port string) string {
	switch port {
	case "993", "465":
		return "SSL/TLS"
	case "143", "587":
		return "STARTTLS"
	}
	return "None — this port is not terminated by anything in this repo"
}

// splitHostPort takes an address the way a listener writes it, where the host
// half is usually empty.
func splitHostPort(addr string) (host, port string) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return "", addr
	}
	return strings.Trim(addr[:i], "[]"), addr[i+1:]
}
