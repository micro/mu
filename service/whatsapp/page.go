package whatsapp

// The page: your WhatsApp threads, and a box to reply.
//
// A reply box rather than a compose box, and the difference is the whole
// service: you may write to somebody for twenty-four hours after they write to
// you, so the page offers the conversations that are open and says when each
// one closes. A field for any number would be a field that mostly refuses.

import (
	"html"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

// Handler serves /whatsapp.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	who := sess.Account

	if r.Method == http.MethodPost {
		handlePost(w, r, who)
		return
	}

	history := History(who, 200)
	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"from": From(), "messages": history})
		return
	}

	var b strings.Builder
	if !Configured() {
		b.WriteString(`<div class="card"><h3>Not set up</h3>` +
			`<p class="text-sm text-muted">This instance has no WhatsApp sender. An operator sets ` +
			`<code>TWILIO_WHATSAPP_FROM</code> at <a href="/admin/env">/admin/env</a> and points the ` +
			`sender's inbound webhook at <code>/whatsapp/twilio</code>.</p></div>`)
		w.Write([]byte(app.RenderHTMLForRequest("WhatsApp", "Reply to people on WhatsApp", b.String(), r)))
		return
	}

	b.WriteString(`<p class="text-sm text-muted">On <strong>` + html.EscapeString(From()) +
		`</strong>. WhatsApp allows a message only within 24 hours of theirs, so these are ` +
		`replies — anybody not here has to write first.</p>`)
	b.WriteString(threads(r, who, history))
	b.WriteString(pageCSS)
	w.Write([]byte(app.RenderHTMLForRequest("WhatsApp", "Reply to people on WhatsApp", b.String(), r)))
}

// threads is one card per conversation, with a reply box on the open ones.
func threads(r *http.Request, who string, history []Message) string {
	if len(history) == 0 {
		return `<div class="card"><p class="text-sm text-muted">Nothing yet. When somebody ` +
			`messages this instance on WhatsApp their conversation appears here.</p></div>`
	}
	csrf := html.EscapeString(auth.CSRFToken(r))

	order := []string{}
	byNumber := map[string][]Message{}
	for _, m := range history {
		if _, ok := byNumber[m.Number]; !ok {
			order = append(order, m.Number)
		}
		byNumber[m.Number] = append(byNumber[m.Number], m)
	}

	var b strings.Builder
	for _, num := range order {
		b.WriteString(`<div class="card"><h3 class="wa-who">` + html.EscapeString(num) + `</h3>`)
		msgs := byNumber[num]
		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			cls := "wa-in"
			if m.Direction == "out" {
				cls = "wa-out"
			}
			b.WriteString(`<div class="wa-msg ` + cls + `">` +
				`<span class="wa-body">` + html.EscapeString(m.Text) + `</span>` +
				`<span class="wa-when">` + html.EscapeString(app.TimeAgo(m.At)) + `</span></div>`)
		}
		if until := OpenUntil(who, num); !until.IsZero() && time.Now().Before(until) {
			b.WriteString(`<form method="POST" action="/whatsapp" class="wa-reply">` +
				`<input type="hidden" name="_csrf" value="` + csrf + `">` +
				`<input type="hidden" name="to" value="` + html.EscapeString(num) + `">` +
				`<input name="text" required maxlength="1500" class="wa-in-box" placeholder="Reply">` +
				`<button type="submit">Send</button></form>` +
				`<p class="wa-until">Open for another ` + html.EscapeString(app.TimeAgo(time.Now().Add(-time.Until(until)))) + `</p>`)
		} else {
			b.WriteString(`<p class="wa-until">Closed — they have to write again before you can reply.</p>`)
		}
		b.WriteString(`</div>`)
	}
	return b.String()
}

func handlePost(w http.ResponseWriter, r *http.Request, who string) {
	if err := r.ParseForm(); err != nil {
		app.Error(w, r, http.StatusBadRequest, "could not read that form")
		return
	}
	if !auth.ValidCSRF(r) {
		app.Error(w, r, http.StatusForbidden, "expired form, try again")
		return
	}
	if _, err := Send(who, r.Form.Get("to"), r.Form.Get("text")); err != nil {
		app.Error(w, r, http.StatusBadRequest, err.Error())
		return
	}
	http.Redirect(w, r, "/whatsapp", http.StatusSeeOther)
}

const pageCSS = `<style>
.wa-who{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:14px;margin:0 0 10px}
.wa-msg{display:flex;flex-direction:column;gap:2px;max-width:min(80%,460px);padding:8px 12px;
  border-radius:12px;margin:0 0 8px}
.wa-out{background:#111;color:#fff;margin-left:auto;border-bottom-right-radius:4px}
.wa-in{background:var(--surface-alt,#f2f2f2);border-bottom-left-radius:4px}
.wa-body{font-size:14px;line-height:1.45;white-space:pre-wrap;word-break:break-word}
.wa-when{font-size:11px;opacity:.65}
.wa-reply{display:flex;gap:8px;margin:10px 0 0}
.wa-in-box{flex:1;padding:8px 10px;border:1px solid var(--border-color,#d1d5db);border-radius:8px;
  font-size:14px;font-family:inherit}
.wa-until{font-size:12px;color:var(--text-muted,#999);margin:8px 0 0}
</style>`
