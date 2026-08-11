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
			"from": Senders(), "messages": history, "yours": Numbers(who),
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

	from := strings.Join(Senders(), " · ")
	if from == "" {
		from = "a number Twilio picks"
	}
	b.WriteString(`<p class="text-sm text-muted">Sent from <strong>` + html.EscapeString(from) +
		`</strong>. ` + html.EscapeString(allowance(who)) + `</p>`)
	b.WriteString(composer(r, who))
	b.WriteString(threads(history))
	b.WriteString(pageCSS)

	w.Write([]byte(app.RenderHTMLForRequest("SMS", "Text somebody, and read what they text back", b.String(), r)))
}

// allowance says what is left, in a sentence rather than a meter.
func allowance(who string) string {
	sent, limit := SentToday(who), LimitFor(who)
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

	// A box you type into, with a datalist of the people you already know
	// beside it. The list was a select, which meant the page could only send
	// where the rules happened to allow — a UI enforcing a rule the service no
	// longer has. Numbers you know are a convenience now, not a gate.
	var b strings.Builder
	b.WriteString(`<div class="card">`)
	b.WriteString(`<form method="POST" action="/sms" class="sms-send">` +
		`<input type="hidden" name="_csrf" value="` + csrf + `">` +
		`<input type="hidden" name="send" value="1">` +
		`<input name="to" class="sms-to" required list="sms-known" autocomplete="off" ` +
		`placeholder="+447700900123" aria-label="Who to text">` +
		`<datalist id="sms-known">`)
	for _, o := range opts {
		b.WriteString(`<option value="` + html.EscapeString(o.number) + `">` +
			html.EscapeString(o.label) + `</option>`)
	}
	b.WriteString(`</datalist>` +
		`<textarea name="text" class="sms-text" rows="3" required maxlength="` + itoa(maxBody) +
		`" placeholder="Say it in 160 characters and it costs one message"></textarea>` +
		`<div><button type="submit">Send</button></div></form>`)

	b.WriteString(verifier(r, who, csrf))
	b.WriteString(`</div>`)
	return b.String()
}

// verifier is claiming a number as your own, in two steps that are two forms.
//
// It was one form — a box for the number, a box for a code, one button — and
// which thing it did depended on whether the second box was empty. Enter a
// number, press "Send code", and if anything at all had found its way into the
// code field you were told to "ask for a code first", about the step you were
// trying to take. A form should not have a mode.
func verifier(r *http.Request, who, csrf string) string {
	number, waiting := Pending(who)

	// Open when there is a code to type in, and open when this account has not
	// verified anything yet — a disclosure triangle is for a thing you are done
	// with, and until one number is yours this is a step, not a footnote.
	var b strings.Builder
	b.WriteString(`<details class="sms-verify"` + openIf(waiting || len(Numbers(who)) == 0) + `>` +
		`<summary>Verify a number as your own</summary>`)

	if waiting {
		b.WriteString(`<p class="text-sm">A code went to <strong>` + html.EscapeString(number) +
			`</strong>. It is good for ten minutes.</p>` +
			`<form method="POST" action="/sms" class="sms-verify-form">` +
			`<input type="hidden" name="_csrf" value="` + csrf + `">` +
			`<input type="hidden" name="confirm" value="` + html.EscapeString(number) + `">` +
			`<input name="code" inputmode="numeric" autocomplete="one-time-code" required ` +
			`placeholder="123456" class="sms-in" aria-label="The code that was texted to you">` +
			`<button type="submit">Confirm</button></form>` +
			`<form method="POST" action="/sms" style="margin:8px 0 0">` +
			`<input type="hidden" name="_csrf" value="` + csrf + `">` +
			`<input type="hidden" name="start" value="` + html.EscapeString(number) + `">` +
			`<button type="submit" class="link-button">Send another code</button></form>`)
	} else {
		b.WriteString(`<form method="POST" action="/sms" class="sms-verify-form">` +
			`<input type="hidden" name="_csrf" value="` + csrf + `">` +
			`<input type="hidden" name="step" value="start">` +
			`<input name="start" required placeholder="+447700900123" class="sms-in" ` +
			`autocomplete="tel" aria-label="Your number">` +
			`<button type="submit">Send code</button></form>` +
			`<p class="text-sm text-muted">We text a code there, charged like any other message. ` +
			`Type it back here and the number is yours — which is what lets a reply to it reach you.</p>`)
	}
	b.WriteString(`</details>`)
	return b.String()
}

func openIf(b bool) string {
	if b {
		return " open"
	}
	return ""
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
	case strings.TrimSpace(r.Form.Get("start")) != "":
		err = StartVerify(who, e164(r.Form.Get("start")))
	case strings.TrimSpace(r.Form.Get("confirm")) != "":
		err = Confirm(who, e164(r.Form.Get("confirm")), r.Form.Get("code"))
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
