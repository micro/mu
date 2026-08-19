package email

import (
	"fmt"
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
		var err error
		done := "?sent=1"
		switch r.FormValue("do") {
		case "verify":
			// One form, two steps: with a code it finishes, without one it starts.
			// Which is showing is decided by Pending, so the page never asks for a
			// code nobody has been sent.
			//
			// The page is the one caller that always means "mine" — a person is
			// looking at their own account — so it claims on approval. The tool
			// does not, because an agent is almost always asking about one of its
			// users' addresses instead.
			addr := r.FormValue("address")
			var v Verification
			if code := strings.TrimSpace(r.FormValue("code")); code != "" {
				if v, err = Check(acc.ID, addr, code); err == nil && v.OK() {
					err = Claim(acc.ID, v.Address)
				}
			} else {
				v, err = StartVerify(acc.ID, addr, "")
			}
			switch {
			case err != nil:
			case v.OK() || v.Status == "pending":
				done = "?said=" + url.QueryEscape(v.Says())
			default:
				// Mistyped, expired, run out of guesses. Not an error the page
				// should style as a failure of the instance, but the person has to
				// be told which of them it was.
				err = fmt.Errorf("%s", v.Says())
			}
		case "forget":
			err, done = Forget(acc.ID, r.FormValue("address")), "?forgot=1"
		default:
			_, err = Send(acc.ID, r.FormValue("to"), r.FormValue("subject"), r.FormValue("body"))
		}
		if err != nil {
			http.Redirect(w, r, "/email?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/email"+done, http.StatusSeeOther)
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
		app.Respond(w, r, app.Response{Title: "Email", Description: "Email that leaves this instance", HTML: b.String()})
		return
	}

	// What this account sends as, and what is left today.
	//
	// Nothing here about replies. Sending from a domain and receiving at one are
	// separate problems with separate DNS, and an MX record is not a thing this
	// service needs to send — that was a consequence of an arrangement that only
	// worked while a Reply-To could be set, and it cannot. A page that warns
	// about it is answering a question nobody has asked yet.
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>You send as</h3>`)
	b.WriteString(`<p style="font-size:15px;margin:0 0 4px"><code>` +
		html.EscapeString(SenderFor(acc.ID)) + `</code></p>`)
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 12px">Carried by ` +
		html.EscapeString(Provider()) + `.</p>`)
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0">` +
		html.EscapeString(strings.ToUpper(Allowance(acc.ID)[:1])+Allowance(acc.ID)[1:]) + `.</p>`)
	b.WriteString(`</div>`)

	note := ""
	switch q := r.URL.Query(); {
	case q.Get("sent") == "1":
		note = "Sent."
	case q.Get("said") != "":
		note = q.Get("said")
	case q.Get("forgot") == "1":
		note = "Removed."
	}
	if note != "" {
		b.WriteString(`<div class="card" style="background:#f0fff0;border-color:#a3d9a5;margin-top:16px"><p style="color:#27ae60;margin:0">` +
			html.EscapeString(note) + `</p></div>`)
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

	// Addresses you have proved you can read.
	//
	// Under sending rather than on the account page, because what it buys is a
	// mail question: this is the only address a stranger can be told to write
	// to, since the domain above has no inbox. The one on the account is shown
	// too, greyed and without a remove button, so the list is every address this
	// instance believes is yours rather than only the ones added here.
	b.WriteString(`<div class="card" style="margin-top:16px"><h3>Your addresses</h3>`)
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 12px">Mail you send from one of these is recognised here — it reaches your agent rather than a spam folder. Nobody can reply to <code>` +
		html.EscapeString(SenderFor(acc.ID)) + `</code>, so this is where to ask for an answer.</p>`)
	yours := Addresses(acc.ID)
	if len(yours) == 0 {
		b.WriteString(`<p style="font-size:14px;color:#888;margin:0 0 12px">None yet.</p>`)
	} else {
		b.WriteString(`<ul style="font-size:14px;margin:0 0 12px;padding-left:18px">`)
		for _, a := range yours {
			b.WriteString(`<li style="margin:0 0 4px"><code>` + html.EscapeString(a) + `</code>`)
			if acc.EmailVerified && strings.EqualFold(acc.Email, a) {
				b.WriteString(` <span style="color:#888">— the address on your account</span>`)
			} else {
				b.WriteString(` <form method="POST" action="/email" style="display:inline">` +
					`<input type="hidden" name="do" value="forget">` +
					`<input type="hidden" name="address" value="` + html.EscapeString(a) + `">` +
					`<button type="submit" style="background:none;border:0;color:#c00;cursor:pointer;font-size:13px;padding:0 0 0 6px">remove</button></form>`)
			}
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`<form method="POST" action="/email"><input type="hidden" name="do" value="verify">`)
	if pending, ok := Pending(acc.ID); ok {
		b.WriteString(`<input type="hidden" name="address" value="` + html.EscapeString(pending) + `">`)
		b.WriteString(`<p style="font-size:14px;color:#666;margin:0 0 8px">A code went to <code>` +
			html.EscapeString(pending) + `</code>.</p>`)
		b.WriteString(`<input name="code" required inputmode="numeric" autocomplete="one-time-code" placeholder="6-digit code" style="width:160px;padding:8px;margin:0 8px 0 0;border:1px solid #ddd;border-radius:6px;font-size:14px">`)
		b.WriteString(`<button type="submit" class="btn">Confirm</button>`)
	} else {
		b.WriteString(`<input name="address" type="email" required placeholder="you@example.com" style="width:260px;padding:8px;margin:0 8px 0 0;border:1px solid #ddd;border-radius:6px;font-size:14px">`)
		b.WriteString(`<button type="submit" class="btn">Send a code</button>`)
		b.WriteString(`<p style="font-size:13px;color:#888;margin:8px 0 0">The code is emailed there, so it costs one send and counts against today's allowance. An agent runs the same check on <em>its</em> users with <code>email_verify</code> — a code under your product's name, and an answer saying whether it came back.</p>`)
	}
	b.WriteString(`</form></div>`)

	b.WriteString(`<div class="card" style="margin-top:16px">`)
	b.WriteString(`<h3>Sent</h3>`)
	msgs := History(acc.ID, 50)
	if len(msgs) == 0 {
		b.WriteString(`<p style="font-size:14px;color:#888;margin:0">Nothing yet. An agent with the <code>email_send</code> tool can send on your behalf.</p>`)
	} else {
		b.WriteString(`<table class="stats-table" style="font-size:14px">`)
		b.WriteString(`<tr><th style="text-align:left">To</th><th style="text-align:left">Subject</th><th style="text-align:left">Result</th><th style="text-align:right">When</th></tr>`)
		for _, m := range msgs {
			// The outcome, in the row. Without it the table answers "what did I
			// try to send" and the question anybody actually has after wiring up
			// a sending domain is whether it went.
			// The carrier's own word, coloured by what it means. "Accepted" is
			// amber on purpose: Twilio answers before anything is delivered, so
			// it is a promise rather than an outcome, and showing it in the same
			// green as "delivered" is what made the history a receipt for one.
			colour := "#888"
			switch m.Status() {
			case "delivered":
				colour = "#27ae60"
			case "undelivered", "failed", "canceled":
				colour = "#c00"
			case "accepted", "sending", "sent", "scheduled":
				colour = "#b8860b"
			}
			result := `<span style="color:` + colour + `">` + html.EscapeString(m.Status()) + `</span>`
			if !m.OK() {
				result = `<span style="color:#c00" title="` + html.EscapeString(m.Error) + `">failed</span>`
			}
			b.WriteString(`<tr><td>` + html.EscapeString(m.To) + `</td><td>` +
				html.EscapeString(clip(m.Subject, 50)) + `</td><td>` + result +
				`</td><td style="text-align:right;color:#888">` +
				m.Sent.Format("2 Jan 15:04") + `</td></tr>`)
		}
		// The reason, under the table, for anything that failed — a title
		// attribute is not something you find unless you already know it is
		// there.
		for _, m := range msgs {
			if !m.OK() {
				b.WriteString(`<p style="font-size:13px;color:#c00;margin:8px 0 0">` +
					html.EscapeString(m.Sent.Format("2 Jan 15:04")) + ` — ` +
					html.EscapeString(m.Error) + `</p>`)
				break
			}
		}
		b.WriteString(`</table>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Email", Description: "Email that leaves this instance", HTML: b.String()})
}
