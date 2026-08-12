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
	"mu/internal/contacts"
	"mu/internal/quota"
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
	b.WriteString(notice(r))
	if !Configured() {
		b.WriteString(`<div class="card"><h3>Not set up</h3>` +
			`<p class="text-sm text-muted">This instance cannot send or receive texts yet. ` +
			`An operator sets these at <a href="/admin/env">/admin/env</a>:</p><ul class="text-sm">`)
		for _, m := range Missing() {
			b.WriteString(`<li><code>` + html.EscapeString(m) + `</code></li>`)
		}
		b.WriteString(`</ul></div>`)
		w.Write([]byte(app.RenderHTMLForRequest("SMS", "Text somebody, and read what they text back", b.String(), r)))
		return
	}

	// Sending and receiving take different credentials, and an instance can be
	// perfectly able to do the first while refusing every reply. Said here
	// because the alternative is finding out from the provider's error log.
	if problem := CanReceive(); problem != "" && verifyInbound() {
		b.WriteString(`<div class="card"><h3>Replies are being refused</h3>` +
			`<p class="text-sm text-muted">Texts go out, and every reply is rejected because it ` +
			`cannot be checked as genuine: ` + html.EscapeString(problem) + `.</p></div>`)
	}

	from := strings.Join(Senders(), " · ")
	if from == "" {
		from = "a number Twilio picks"
	}
	// Which numbers are yours, on the page rather than inside the fold. It was
	// only visible with the verification section open, so somebody who had
	// verified a number and closed it had no way to see that they had, and no
	// way to tell why a message from that number went somewhere else.
	yours := ""
	if mine := Numbers(who); len(mine) > 0 {
		yours = " Yours: " + strings.Join(mine, ", ") + "."
	}
	b.WriteString(`<p class="text-sm text-muted">Sent from <strong>` + html.EscapeString(from) +
		`</strong>. ` + html.EscapeString(allowance(who)+yours) + `</p>`)
	b.WriteString(composer(r, who))
	b.WriteString(threads(r, who, history))
	b.WriteString(pageCSS)

	w.Write([]byte(app.RenderHTMLForRequest("SMS", "Text somebody, and read what they text back", b.String(), r)))
}

// notice says what just happened.
//
// Everything here redirected to /sms and said nothing, so a verification that
// worked and one that failed looked identical: the page came back, the section
// collapsed, and nowhere on it said which numbers were yours. Somebody who had
// just verified a number could only find out by trying again and being told it
// was already theirs.
//
// Keyed rather than free text — the message is ours, so the URL cannot put
// words in the page's mouth.
func notice(r *http.Request) string {
	msg := map[string]string{
		"sent":     "Sent.",
		"code":     "Code sent. Type it in below.",
		"verified": "That number is yours now.",
		"forgot":   "That number is no longer yours.",
	}[r.URL.Query().Get("ok")]
	if msg == "" {
		return ""
	}
	return `<div class="notice"><p>` + html.EscapeString(msg) + `</p></div>`
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
		// What is already yours, said out loud. It was only ever visible as a
		// line in the recipient autocomplete, which is not a place anybody
		// looks to answer "did that work".
		if mine := Numbers(who); len(mine) > 0 {
			b.WriteString(`<p class="text-sm">Yours: `)
			for i, n := range mine {
				if i > 0 {
					b.WriteString(` · `)
				}
				b.WriteString(`<strong>` + html.EscapeString(n) + `</strong> ` +
					`<a href="#" onclick="document.getElementById('forget-` +
					html.EscapeString(n) + `').submit();return false;" class="link">remove</a>`)
			}
			b.WriteString(`</p>`)
			for _, n := range mine {
				b.WriteString(`<form method="POST" action="/sms" id="forget-` +
					html.EscapeString(n) + `" style="display:none">` +
					`<input type="hidden" name="_csrf" value="` + csrf + `">` +
					`<input type="hidden" name="forget" value="` + html.EscapeString(n) + `"></form>`)
			}
		}
		b.WriteString(`<form method="POST" action="/sms" class="sms-verify-form">` +
			`<input type="hidden" name="_csrf" value="` + csrf + `">` +
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

// threads is the history, grouped by the other end, each with a box to reply in.
//
// The reply box is the point. Every conversation was read-only: to answer
// somebody you scrolled back to the compose box at the top and typed their
// number in again, having just read it. WhatsApp put the box in the thread from
// the start and this did not, which is the same service wearing two different
// manners.
//
// The compose box stays, because SMS can start a conversation and WhatsApp
// cannot — that is the real difference between them, and it is the only one
// worth showing.
func threads(r *http.Request, who string, history []Message) string {
	if len(history) == 0 {
		return ""
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
		// Somebody who has said STOP has said it to us, and the page should say
		// so where the reply box would be rather than take a message and fail.
		if OptedOut(number) {
			b.WriteString(`<p class="sms-closed">They asked not to be texted. ` +
				`Nothing more goes to this number.</p>`)
		} else {
			b.WriteString(`<form method="POST" action="/sms" class="sms-reply">` +
				`<input type="hidden" name="_csrf" value="` + csrf + `">` +
				`<input type="hidden" name="send" value="1">` +
				`<input type="hidden" name="to" value="` + html.EscapeString(number) + `">` +
				`<input name="text" required maxlength="` + itoa(maxBody) + `" class="sms-reply-box" ` +
				`placeholder="Reply" aria-label="Reply to ` + html.EscapeString(number) + `">` +
				`<button type="submit">Send</button></form>`)
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
	done := ""
	switch {
	case r.Form.Get("send") != "":
		_, err = Send(who, r.Form.Get("to"), r.Form.Get("text"))
		done = "sent"
	case strings.TrimSpace(r.Form.Get("start")) != "":
		err = StartVerify(who, e164(r.Form.Get("start")))
		done = "code"
	case strings.TrimSpace(r.Form.Get("confirm")) != "":
		err = Confirm(who, e164(r.Form.Get("confirm")), r.Form.Get("code"))
		done = "verified"
	case strings.TrimSpace(r.Form.Get("forget")) != "":
		Forget(who, r.Form.Get("forget"))
		done = "forgot"
	}
	if err != nil {
		app.Error(w, r, http.StatusBadRequest, err.Error())
		return
	}
	http.Redirect(w, r, "/sms?ok="+done, http.StatusSeeOther)
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
.sms-reply{display:flex;gap:8px;margin:10px 0 0}
.sms-reply-box{flex:1;min-width:0;padding:8px 10px;border:1px solid var(--border-color,#d1d5db);
  border-radius:8px;font-size:14px;font-family:inherit}
.sms-closed{font-size:12px;color:var(--text-muted,#999);margin:8px 0 0}
</style>`
