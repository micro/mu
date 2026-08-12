package email

import (
	"html"
	"net/http"
	"net/url"
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

// Handler is /email: what you send as, what is left today, what has gone, and a
// box to send one.
//
// The box was left out on the argument that this is not a second mailbox and a
// form would invite treating it as one. That was wrong in the ordinary way: a
// service with a page and no way to do the thing it is for cannot be tried. An
// operator setting up a sending domain needs to send one message to know
// whether it works, and telling them to go and write an MCP call for that is
// telling them to debug two things at once.
func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		http.Redirect(w, r, "/login?next=/email", http.StatusSeeOther)
		return
	}

	// Sending goes through the same Send the tool calls, so the page cannot
	// take a different route past the caps or the charge — the mistake this
	// whole billing effort was about.
	if r.Method == "POST" {
		r.ParseForm() //nolint:errcheck
		_, err := Send(acc.ID, r.FormValue("to"), r.FormValue("subject"), r.FormValue("body"))
		if err != nil {
			http.Redirect(w, r, "/email?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/email?sent=1", http.StatusSeeOther)
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
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0">` +
		html.EscapeString(strings.ToUpper(Allowance(acc.ID)[:1])+Allowance(acc.ID)[1:]) + `.</p>`)
	b.WriteString(`</div>`)

	if r.URL.Query().Get("sent") == "1" {
		b.WriteString(`<div class="card" style="background:#f0fff0;border-color:#a3d9a5;margin-top:16px"><p style="color:#27ae60;margin:0">Sent.</p></div>`)
	}
	if msg := r.URL.Query().Get("error"); msg != "" {
		b.WriteString(`<div class="card" style="border-color:#e0a3a3;margin-top:16px"><p style="color:#c00;margin:0">` +
			html.EscapeString(msg) + `</p></div>`)
	}

	// One message, to see that it works.
	b.WriteString(`<div class="card" style="margin-top:16px"><h3>Send one</h3>`)
	b.WriteString(`<form method="POST" action="/email">`)
	b.WriteString(`<input name="to" type="email" required placeholder="to@example.com" style="width:100%;padding:8px;margin:0 0 8px;border:1px solid #ddd;border-radius:6px;font-size:14px;box-sizing:border-box">`)
	b.WriteString(`<input name="subject" required placeholder="Subject" style="width:100%;padding:8px;margin:0 0 8px;border:1px solid #ddd;border-radius:6px;font-size:14px;box-sizing:border-box">`)
	b.WriteString(`<textarea name="body" required rows="6" placeholder="Message" style="width:100%;padding:8px;margin:0 0 8px;border:1px solid #ddd;border-radius:6px;font-size:14px;box-sizing:border-box;font-family:inherit"></textarea>`)
	b.WriteString(`<button type="submit" class="btn">Send</button>`)
	b.WriteString(`</form></div>`)

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
