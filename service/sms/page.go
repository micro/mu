package sms

// The page: your texts, and a box to send one.
//
// Grouped by the other end rather than listed flat, because a text is half of a
// conversation and a chronological list of both directions from six people is
// unreadable. The compose box only offers numbers you may actually text, which
// is the same rule the tool enforces said in the UI instead of in an error.

import (
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/service/contacts"
)

// Handler serves /sms.
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
		app.RespondJSON(w, map[string]any{
			"number": From(), "messages": history, "yours": Numbers(who),
		})
		return
	}

	var b strings.Builder
	if !Configured() {
		b.WriteString(`<div class="card"><h3>No number</h3>` +
			`<p class="text-sm text-muted">This instance has no phone number configured, so it ` +
			`cannot send or receive texts. An operator sets <code>TWILIO_ACCOUNT_SID</code>, ` +
			`<code>TWILIO_AUTH_TOKEN</code> and <code>TWILIO_FROM</code>.</p></div>`)
		w.Write([]byte(app.RenderHTMLForRequest("SMS", "Text somebody, and read what they text back", b.String(), r)))
		return
	}

	b.WriteString(`<p class="text-sm text-muted">Sent from <strong>` + html.EscapeString(From()) +
		`</strong>. ` + html.EscapeString(allowance(who)) + `</p>`)
	b.WriteString(composer(r, who))
	b.WriteString(threads(history))
	b.WriteString(pageCSS)

	w.Write([]byte(app.RenderHTMLForRequest("SMS", "Text somebody, and read what they text back", b.String(), r)))
}

// allowance says what is left, in a sentence rather than a meter.
func allowance(who string) string {
	sent, limit := SentToday(who), DailyLimit()
	left := limit - sent
	if left < 0 {
		left = 0
	}
	s := "s"
	if left == 1 {
		s = ""
	}
	msg := "" + itoa(left) + " message" + s + " left today"
	if cost := quota.GetOperationCost(quota.OpSMSSend); cost > 0 {
		msg += ", at " + itoa(cost) + " credits each"
	}
	return msg + "."
}

// composer is the send box. Its recipient list is the rule, rendered.
func composer(r *http.Request, who string) string {
	csrf := html.EscapeString(auth.CSRFToken(r))

	type option struct{ number, label string }
	var opts []option
	seen := map[string]bool{}
	for _, c := range contacts.List(who) {
		n := e164(c.Phone)
		if n == "" || seen[n] || OptedOut(n) {
			continue
		}
		seen[n] = true
		opts = append(opts, option{n, c.Name + " · " + n})
	}
	for _, n := range Numbers(who) {
		if !seen[n] && !OptedOut(n) {
			seen[n] = true
			opts = append(opts, option{n, n + " · yours"})
		}
	}
	for _, m := range History(who, 200) {
		if m.Direction == "in" && !seen[m.Number] && !OptedOut(m.Number) {
			seen[m.Number] = true
			opts = append(opts, option{m.Number, m.Number + " · texted you"})
		}
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].label < opts[j].label })

	var b strings.Builder
	b.WriteString(`<div class="card">`)
	if len(opts) == 0 {
		// The empty state is the rule stated plainly, which is better than
		// letting somebody type a number and be refused.
		b.WriteString(`<p class="text-sm text-muted">There is nobody to text yet. You can text ` +
			`someone with a number in <a href="/contacts">your contacts</a>, a number you have ` +
			`verified as your own, or anyone who texts this number first.</p>`)
	} else {
		b.WriteString(`<form method="POST" action="/sms" class="sms-send">` +
			`<input type="hidden" name="_csrf" value="` + csrf + `">` +
			`<input type="hidden" name="send" value="1">` +
			`<select name="to" class="sms-to" aria-label="Who to text">`)
		for _, o := range opts {
			b.WriteString(`<option value="` + html.EscapeString(o.number) + `">` +
				html.EscapeString(o.label) + `</option>`)
		}
		b.WriteString(`</select>` +
			`<textarea name="text" class="sms-text" rows="3" required maxlength="` + itoa(maxBody) +
			`" placeholder="Say it in 160 characters and it costs one message"></textarea>` +
			`<div><button type="submit">Send</button></div></form>`)
	}

	// Verifying your own number, folded in rather than given a page: it is done
	// once and never thought about again.
	b.WriteString(`<details class="sms-verify"><summary>Verify a number as your own</summary>` +
		`<form method="POST" action="/sms" class="sms-verify-form">` +
		`<input type="hidden" name="_csrf" value="` + csrf + `">` +
		`<input name="verify" placeholder="+447700900123" class="sms-in" aria-label="Your number">` +
		`<input name="code" placeholder="code, if you have one" class="sms-in" aria-label="The code that was texted to you">` +
		`<button type="submit">Send code</button></form>` +
		`<p class="text-sm text-muted">A code is texted there and charged like any other message. ` +
		`Send the number on its own first, then the number and the code together.</p></details>`)
	b.WriteString(`</div>`)
	return b.String()
}

// threads is the history, grouped by the other end.
func threads(history []Message) string {
	if len(history) == 0 {
		return ""
	}
	order := []string{}
	byNumber := map[string][]Message{}
	for _, m := range history {
		if _, ok := byNumber[m.Number]; !ok {
			order = append(order, m.Number)
		}
		byNumber[m.Number] = append(byNumber[m.Number], m)
	}

	var b strings.Builder
	for _, number := range order {
		b.WriteString(`<div class="card"><h3 class="sms-who">` + html.EscapeString(number) + `</h3>`)
		msgs := byNumber[number]
		// Oldest first inside a conversation, which is how a conversation reads.
		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			cls := "sms-in-msg"
			if m.Direction == "out" {
				cls = "sms-out-msg"
			}
			b.WriteString(`<div class="sms-msg ` + cls + `">` +
				`<span class="sms-body">` + html.EscapeString(m.Text) + `</span>` +
				`<span class="sms-when">` + html.EscapeString(app.TimeAgo(m.At)) + `</span></div>`)
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

	var err error
	switch {
	case r.Form.Get("send") != "":
		_, err = Send(who, r.Form.Get("to"), r.Form.Get("text"))
	case strings.TrimSpace(r.Form.Get("verify")) != "":
		number := r.Form.Get("verify")
		if code := strings.TrimSpace(r.Form.Get("code")); code != "" {
			err = Confirm(who, e164(number), code)
		} else {
			err = StartVerify(who, e164(number))
		}
	}
	if err != nil {
		app.Error(w, r, http.StatusBadRequest, err.Error())
		return
	}
	http.Redirect(w, r, "/sms", http.StatusSeeOther)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	if neg {
		return "-" + string(d)
	}
	return string(d)
}

const pageCSS = `<style>
.sms-send{display:flex;flex-direction:column;gap:8px}
.sms-to,.sms-in{padding:8px 10px;border:1px solid var(--border-color,#d1d5db);border-radius:8px;
  font-size:14px;font-family:inherit}
.sms-text{padding:10px;border:1px solid var(--border-color,#d1d5db);border-radius:8px;font-size:14px;
  font-family:inherit;line-height:1.5;resize:vertical}
.sms-verify{margin-top:14px}
.sms-verify summary{font-size:13px;color:var(--text-muted,#666);cursor:pointer}
.sms-verify-form{display:flex;flex-wrap:wrap;gap:8px;align-items:center;margin:10px 0 0}
.sms-who{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:14px;margin:0 0 10px}
.sms-msg{display:flex;flex-direction:column;gap:2px;max-width:min(80%,460px);padding:8px 12px;
  border-radius:12px;margin:0 0 8px}
.sms-out-msg{background:#111;color:#fff;margin-left:auto;border-bottom-right-radius:4px}
.sms-in-msg{background:var(--surface-alt,#f2f2f2);border-bottom-left-radius:4px}
.sms-body{font-size:14px;line-height:1.45;white-space:pre-wrap;word-break:break-word}
.sms-when{font-size:11px;opacity:.65}
</style>`
