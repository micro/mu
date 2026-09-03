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

	"mu/agent"
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

// VCardHandler serves /contact.vcf — the agent as an address-book entry.
//
// A file rather than a page, and the Content-Type is what does the work: a
// phone offered text/vcard opens its contacts app and asks whether to save.
// Served to anybody, because everything in it is already on the page above it.
func VCardHandler(w http.ResponseWriter, r *http.Request) {
	if !client.Savable() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	// The filename a phone shows while it asks. inline rather than attachment:
	// a phone that downloads this to a Files app instead of opening it has put
	// the contact somewhere nobody will look again.
	w.Header().Set("Content-Disposition", `inline; filename="`+vcardName()+`.vcf"`)
	w.Header().Set("Cache-Control", "no-cache, private")
	w.Write([]byte(client.VCard(agent.DefaultName()))) //nolint:errcheck
}

// vcardName is the file's name: the agent's, lowercased, with nothing in it
// that a filesystem would argue about.
func vcardName() string {
	var out strings.Builder
	for _, r := range strings.ToLower(agent.DefaultName()) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			out.WriteRune(r)
		}
	}
	if out.Len() == 0 {
		return "assistant"
	}
	return out.String()
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
		b.WriteString(`<span class="cnote">` + html.EscapeString(c.Note) + `</span>`)
		// The call itself, for the one row that is an instruction rather than
		// an address. See client.Client.Example.
		if c.Example != "" {
			b.WriteString(`<pre class="cex">` + html.EscapeString(c.Example) + `</pre>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	// And the card itself, for a phone.
	//
	// Everything above is four things to copy out by hand, and nobody does
	// that: the point of an assistant you can text is that texting it is the
	// easy thing, and it is only easy once it is in the list where your phone
	// keeps the people you write to. One tap saves it there under one name,
	// with every way of reaching it under that name.
	//
	// Only when there is something a phone can hold. On an instance with no
	// number and no mail domain the card would be a name and a URL, which is a
	// bookmark, and the button would be a promise of more than it does.
	if client.Savable() {
		b.WriteString(`<p class="mt-4">` + app.ActionLink("/contact.vcf", "Add to contacts") +
			`</p><p class="ccap">Saves ` + html.EscapeString(agent.DefaultName()) +
			` to your phone with every number and address on it.</p>`)
	}

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
/* The worked example, under the row it belongs to. Scrolls inside itself
   rather than widening the card: a curl line with a URL in it is longer than
   any column this page has. */
.cex{grid-column:2;margin:8px 0 0;padding:10px 12px;background:#fafafa;
  border:1px solid #f0f0f0;border-radius:6px;font-size:12px;line-height:1.6;
  overflow-x:auto;white-space:pre;color:#333}
@media (max-width:520px){
  .crow{grid-template-columns:1fr}
  .cnote,.caddr,.cex{grid-column:1}
}
</style>`
