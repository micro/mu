package home

// How to reach the assistant.
//
// This is one page because there is one question behind five scattered answers.
// The phone number was on /sms, the WhatsApp number on the same page behind a
// pill, the mail address on /agents and in the Connect panel, the curl line on
// /api, and the web box is wherever you already are. Somebody who wanted to know
// "how do I actually use this thing" had to visit four pages and already know
// which four.
//
// # Clients, and where the list of them lives
//
// A client is how you reach the agent, as against a service, which is what the
// agent reaches. internal/thread already names them — WebClient, SMSClient,
// WhatsAppClient, ChatClient, and mail.Client beside its own door — because the
// record has to say which door a conversation came through. What did not exist
// was a *list*: five constants in two packages, and nothing that could
// enumerate them or say what this instance's address is on each.
//
// The addresses are why the list is here and not down beside the constants.
// They come from service/sms (the numbers, per channel), service/mail (the
// domain) and the settings (the host), and a service may not import another
// service — so nothing under internal/ or service/ can assemble them. A page
// package may, which is the same reason the doors row is built here from
// service.Guest.
//
// So the list lives in mu/client, which is the package that names the other
// half of what Go Micro started with: a service is a name with registered
// handlers, and a client is a way to reach one. This page renders it.

import (
	"html"
	"net/http"
	"strings"

	"mu/client"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/sms"
)

// ContactHandler serves the card.
//
// Public, and deliberately. Every address on it is one this instance publishes
// anyway — a number that answers texts, an address that answers mail — and the
// page exists so a stranger can find out what this is before deciding to have
// an account. Gating "how to contact us" behind signing in is the shape of
// question this product is supposed to be the answer to.
func ContactHandler(w http.ResponseWriter, r *http.Request) {
	var acc *auth.Account
	if _, a := auth.TrySession(r); a != nil {
		acc = a
	}
	app.Respond(w, r, app.Response{
		Title:       "Contact",
		Description: "Every way to reach this instance's assistant — the web, a text, WhatsApp, mail, or a program.",
		HTML:        contactBody(acc),
	})
}

// contactBody is the card, separate from serving it.
func contactBody(acc *auth.Account) string {
	var b strings.Builder
	b.WriteString(contactCSS)
	b.WriteString(`<div class="card"><h3>How to reach Micro</h3>`)
	b.WriteString(`<p class="ccap">The same assistant, the same memory, whichever way you write. ` +
		`A conversation you start by text is one you can carry on here.</p>`)
	b.WriteString(`<div class="clist">`)
	for _, c := range client.All() {
		b.WriteString(`<div class="crow"><span class="clabel">` + html.EscapeString(c.Label) + `</span>`)
		addr := `<code class="caddr">` + html.EscapeString(c.Address) + `</code>`
		if c.Href != "" {
			addr = `<a class="caddr" href="` + html.EscapeString(c.Href) + `">` +
				html.EscapeString(c.Address) + `</a>`
		}
		b.WriteString(addr)
		b.WriteString(`<span class="cnote">` + html.EscapeString(c.Note) + `</span></div>`)
	}
	b.WriteString(`</div>`)

	// What it needs from you before a phone number is you rather than a
	// stranger's. Only where it is actionable: signed out there is no account
	// to attach a number to, and the honest next step is an account.
	if acc == nil {
		b.WriteString(`<p class="cnext">Texting works once it knows the number is yours — ` +
			`<a href="/signup">make an account</a> and verify it.</p>`)
	} else if !numberVerified(acc.ID) {
		b.WriteString(`<p class="cnext">It will not recognise you by phone until you have ` +
			`<a href="/sms">verified a number</a> as yours. Mail and the web already know you.</p>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// numberVerified reports whether this account has proved a number is theirs,
// which is what the phone clients recognise somebody by.
func numberVerified(accountID string) bool {
	return len(sms.Numbers(accountID)) > 0
}

// contactCSS is the card's own, because it is the only thing shaped like this.
//
// A definition list would be the semantic answer and reads badly at this width:
// the label, the address and the note are three columns on a desktop and three
// stacked lines on a phone, which is a grid.
const contactCSS = `<style>
.clist{display:flex;flex-direction:column;gap:2px;margin:14px 0 0}
.crow{display:grid;grid-template-columns:90px minmax(0,1fr);gap:2px 14px;
  align-items:baseline;padding:10px 0;border-top:1px solid #f0f0f0}
.crow:first-child{border-top:0}
.clabel{font-weight:700;font-size:14px}
.caddr{font-size:15px;color:#111;text-decoration:none;word-break:break-all}
a.caddr{color:#0645ad}
a.caddr:hover{text-decoration:underline}
.cnote{grid-column:2;font-size:13px;color:#888}
.ccap{color:#666;font-size:14px;line-height:1.5;margin:0}
.cnext{margin:16px 0 0;font-size:14px;color:#666}
@media (max-width:520px){
  .crow{grid-template-columns:1fr}
  .cnote,.caddr{grid-column:1}
}
</style>`
