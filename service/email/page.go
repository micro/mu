package email

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// Load registers email as a service and puts its page up.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("email", "service register failed: %v", err)
	}
	http.HandleFunc("/email", Handler)
}

// Handler is /email: what you send as, what is left today, and what has gone.
//
// No compose form. Sending from a page is what mail already offers and this is
// not a second mailbox — the page exists to answer the two questions somebody
// has before they let an agent send on their behalf: where does it come from,
// and what has it done so far. A form would invite using this as a mail client,
// which it is not.
func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		http.Redirect(w, r, "/login?next=/email", http.StatusSeeOther)
		return
	}

	var b strings.Builder
	b.WriteString(`<div style="max-width:760px">`)

	if miss := Missing(); len(miss) > 0 {
		b.WriteString(`<div class="card"><h3>Not configured</h3>`)
		b.WriteString(`<p style="font-size:14px;color:#666">Nothing can be sent until an operator sets these at <a href="/admin/env">/admin/env</a>:</p><ul style="font-size:14px;color:#666">`)
		for _, m := range miss {
			b.WriteString(`<li>` + html.EscapeString(m) + `</li>`)
		}
		b.WriteString(`</ul></div></div>`)
		w.Write([]byte(app.RenderHTMLForRequest("Email", "Email that leaves this instance", b.String(), r))) //nolint:errcheck
		return
	}

	// Where it comes from and where answers land. The second line is the one
	// worth putting on a page: mail goes out from a domain with no inbox, so
	// "reply to this" resolves somewhere else, and somebody should not have to
	// discover that from a bounce.
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>You send as</h3>`)
	b.WriteString(`<p style="font-size:15px;margin:0 0 4px"><code>` +
		html.EscapeString(SenderFor(acc.ID)) + `</code></p>`)
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 12px">Replies arrive at <code>` +
		html.EscapeString(ReplyFor(acc.ID)) + `</code>, which is your inbox here — the sending domain carries no mail in.</p>`)
	b.WriteString(fmt.Sprintf(`<p style="font-size:14px;color:#666;margin:0">%d of %d left today.</p>`,
		LeftToday(acc.ID), LimitFor(acc.ID)))
	b.WriteString(`</div>`)

	b.WriteString(`<div class="card" style="margin-top:16px">`)
	b.WriteString(`<h3>Sent</h3>`)
	msgs := History(acc.ID, 50)
	if len(msgs) == 0 {
		b.WriteString(`<p style="font-size:14px;color:#888;margin:0">Nothing yet. An agent with the <code>email_send</code> tool can send on your behalf.</p>`)
	} else {
		b.WriteString(`<table class="stats-table" style="font-size:14px">`)
		b.WriteString(`<tr><th style="text-align:left">To</th><th style="text-align:left">Subject</th><th style="text-align:right">When</th></tr>`)
		for _, m := range msgs {
			b.WriteString(`<tr><td>` + html.EscapeString(m.To) + `</td><td>` +
				html.EscapeString(clip(m.Subject, 60)) + `</td><td style="text-align:right;color:#888">` +
				m.Sent.Format("2 Jan 15:04") + `</td></tr>`)
		}
		b.WriteString(`</table>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
	w.Write([]byte(app.RenderHTMLForRequest("Email", "Email that leaves this instance", b.String(), r))) //nolint:errcheck
}
