package home

// About describes the assistant in product terms.

import (
	"net/http"
	"strings"

	"mu/internal/app"
)

func AboutHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(app.Column())
	b.WriteString(`<div class="card">` +
		`<p>Micro is a personal assistant. You write to it the way you would write to a ` +
		`person — from the web, by email, by text, on WhatsApp, or from a program — and it ` +
		`answers, remembers, and does things on your behalf. It reads your mail, searches ` +
		`the web, checks the weather and the markets, keeps your notes and your calendar. ` +
		`Same assistant and the same memory whichever way you reach it, so a conversation ` +
		`you start on your phone is one you can carry on in a browser.</p>` +
		`</div>`)

	b.WriteString(app.Close())

	app.RespondPublic(w, r, app.Response{
		Title:       "About Micro",
		Description: "Micro is a personal assistant powered by the open source Mu runtime.",
		HTML:        b.String(),
	})
}
