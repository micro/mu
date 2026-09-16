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
	"encoding/json"
	"html"
	"mu/internal/app"
	"net/http"
	"strings"

	"mu/agent"
	"mu/internal/auth"
	"mu/internal/client"
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
	if app.WantsJSON(r) {
		contactHandlerJSON(w, r)
		return
	}
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
		return "agent"
	}
	return out.String()
}

// numberVerified reports whether this account has proved a number is theirs,
// which is what the phone clients recognise somebody by.
func numberVerified(accountID string) bool {
	return len(sms.Numbers(accountID)) > 0
}

// "" is the card's own, because it is the only thing shaped like this.
//
// A definition list would be the semantic answer and reads badly at this width:
// the label, the address and the note are three columns on a desktop and three
// stacked lines on a phone, which is a grid.

func contactHandlerJSON(w http.ResponseWriter, r *http.Request) {
	var acc *auth.Account
	if _, a := auth.TrySession(r); a != nil {
		acc = a
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	json.NewEncoder(w).Encode(map[string]any{"channels": client.Personal(), "savable": client.Savable(), "verify_number": acc != nil && !numberVerified(acc.ID)})
}

func contactBody(acc *auth.Account) string {
	var b strings.Builder
	// The same column every other page of its kind uses.
	//
	// This drew a bare card, so its content sat against the left of the content
	// box while /about and /privacy — which do use the column — sat centred
	// under a collapsed rail. Four pages a stranger reads in one sitting, at
	// three different widths and two different left edges. See app.Column.
	b.WriteString(app.Column())
	b.WriteString(`<div class="card"><h3>How to reach Micro</h3>`)
	b.WriteString(`<p class="ccap">The same assistant, the same memory, whichever way you write. ` +
		`A conversation you start by text is one you can carry on here.</p>`)
	b.WriteString(`<div class="clist">`)
	// The ways a person writes to it, and not the ways a program calls it.
	//
	// This drew client.All(), which ends in `mu ask "…"` and a curl invocation
	// with a bearer token in it — so a card headed "How to reach Micro", whose
	// whole argument is that you can text this thing like a person, finished
	// with a shell snippet. Those are answers to a different question and /api
	// is where it is asked. See client.Personal.
	for _, c := range client.Personal() {
		b.WriteString(`<div class="crow"><span class="clabel">` + html.EscapeString(c.Label) + `</span>`)
		addr := `<code class="caddr">` + html.EscapeString(c.Address) + `</code>`
		if c.Href != "" {
			addr = `<a class="caddr" href="` + html.EscapeString(c.Href) + `">` +
				html.EscapeString(c.Address) + `</a>`
		}
		b.WriteString(addr)
		b.WriteString(`<span class="cnote">` + html.EscapeString(c.Note) + `</span>`)
		// No worked example here any more. The one row that needed one was the
		// API, which is a developer door and is drawn on /api instead — see
		// client.Developer. Every row left is an address: somebody reads one and
		// knows what to do with it.
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

	// What it needs from you before any of these answer.
	//
	// This said texting needs an account and left mail alone, so the Email row
	// above it — "write to it and it writes back" — was a promise this instance
	// does not keep for the reader it was written for. A stranger who mails
	// agent@ is dropped without a reply and without a record: see
	// service/mail/smtp.go, where AccountForVerifiedEmail returns nothing and
	// the message is discarded. A stranger who texts the number is filed and
	// not answered, because service/sms only wakes an agent for a sender the
	// account knows.
	//
	// So the caveat covers every row except the web, which is the one door a
	// guest really can walk through — the box on the front page answers without
	// an account, bounded. Naming the exception is what keeps this a fact rather
	// than a wall: there is something you can try right now, and the rest is
	// what an account is for.
	if acc == nil {
		b.WriteString(`<p class="cnext">These answer once it knows who you are. ` +
			`The box on the <a href="/">front page</a> works without an account — ` +
			`for the rest, <a href="/signup">make one</a> and verify your number.</p>`)
	} else if !numberVerified(acc.ID) {
		b.WriteString(`<p class="cnext">It will not recognise you by phone until you have ` +
			`<a href="/sms">verified a number</a> as yours. Mail and the web already know you.</p>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(app.Close())
	return b.String()
}
