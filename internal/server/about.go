package server

// What Micro does and the software behind it.
//
// Micro is the person-facing assistant. Mu is the runtime underneath it.
// Keeping that distinction here matters because this is the one explanatory
// page a stranger may read before using the product.

import (
	"net/http"
	"strings"

	"mu/internal/app"
)

func AboutHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(app.Column())
	b.WriteString(`<div class="card"><h3>What Micro is</h3>` +
		`<p>Micro is a personal assistant for everyday questions, planning and getting things done. ` +
		`Talk to it on the web, by email, WhatsApp or XMPP. It can search for information, ` +
		`work with your notes, files and tasks, and create events and reminders. ` +
		`Connect Google Calendar to include your schedule.</p>` +
		`<p>You can also opt into a morning brief, a daily checkin or scheduled topic updates.</p>` +
		`<p>Micro runs on <a href="https://github.com/micro/mu">Mu</a>, the open source software ` +
		`behind its agents, services and inbox. You can run Mu yourself, with Micro as the default assistant.</p>` +
		`</div>`)

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{
		Title:       "About",
		Description: "Micro is a personal assistant powered by the open source Mu runtime.",
		HTML:        b.String(),
	})
}
