package home

// What this is, in two paragraphs.
//
// Micro is the person-facing assistant. Mu is the runtime underneath it.
// Keeping that distinction here matters because this is the one explanatory
// page a stranger may read before using the product.

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/service/mail"
)

func AboutHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(app.Column())
	b.WriteString(`<div class="card"><h3>What Micro is</h3>` +
		`<p>Micro is a personal assistant. You write to it the way you would write to a ` +
		`person — from the web, by email, by text, on WhatsApp, or from a program — and it ` +
		`answers, remembers, and does things on your behalf. It reads your mail, searches ` +
		`the web, checks the weather and the markets, keeps your notes and your calendar. ` +
		`Same assistant and the same memory whichever way you reach it, so a conversation ` +
		`you start on your phone is one you can carry on in a browser.</p>` +
		`<p>Micro runs on <a href="https://github.com/micro/mu">Mu</a>, the open source runtime behind the assistant: the services, ` +
		`archive, inbox and agent system that make those capabilities available. You can ` +
		`run Mu yourself and Micro remains the default agent and front door.</p>` +
		`</div>`)

	if address := mail.HelloAddress(); address != "" {
		b.WriteString(`<p>Say hello: <a href="mailto:` + html.EscapeString(address) + `">` + html.EscapeString(address) + `</a></p>`)
	}

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{
		Title:       "About",
		Description: "Micro is a personal assistant powered by the open source Mu runtime.",
		HTML:        b.String(),
	})
}
