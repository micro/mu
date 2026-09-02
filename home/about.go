package home

// What this is, in two paragraphs.
//
// /about used to exist and was deleted, correctly: it was a landing page's
// pitch — a headline, a positioning paragraph, three cards and a payment
// explanation — on a server that ships to people who have already installed it.
// Marketing is a thing an instance may serve; it is not a thing the software
// should contain.
//
// This is the other thing, and it is the one the footer was missing. A stranger
// who lands on a page here and wants to know what they are looking at had
// nowhere to go: Archive is a corpus, Tools is a catalogue, Contact is a list of
// addresses. Every one of them answers a question you can only ask once you know
// what this is.
//
// So: two short paragraphs, no argument, nothing to weigh. It says what the
// thing does and what it does not do, and stops. The test of a page like this is
// whether removing the last sentence would make it worse; if a sentence is
// there to persuade rather than to inform, it goes.

import (
	"net/http"
	"strings"

	"mu/internal/app"
)

func AboutHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(app.Column())
	b.WriteString(`<div class="card"><h3>What Mu is</h3>` +
		`<p>Mu is a personal assistant. You write to it the way you would write to a ` +
		`person — from the web, by email, by text, on WhatsApp, or from a program — and it ` +
		`answers, remembers, and does things on your behalf. It reads your mail, searches ` +
		`the web, checks the weather and the markets, keeps your notes and your calendar. ` +
		`Same assistant and the same memory whichever way you reach it, so a conversation ` +
		`you start on your phone is one you can carry on in a browser.</p>` +
		`<p>There are no ads, no algorithmic feed and no tracking, and that is a design ` +
		`constraint rather than a promise: nothing here is paid for by your attention, so ` +
		`there is nothing to gain by keeping you on the page. You ask for what you need and ` +
		`then you leave. It is open source, and you can run the whole thing yourself.</p>` +
		`</div>`)
	b.WriteString(`<div class="card"><h3>Where to go next</h3><p class="text-sm">` +
		`<a href="/contact">Every way to reach it</a> · ` +
		`<a href="/archive">What this server has read</a> · ` +
		`<a href="/api">For programs</a> · ` +
		`<a href="https://github.com/micro/mu">The source</a>` +
		`</p></div>`)

	app.Respond(w, r, app.Response{
		Title:       "About",
		Description: "Mu is a personal assistant you can reach from the web, email, SMS, WhatsApp or a program.",
		HTML:        b.String(),
	})
}
