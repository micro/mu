package notify

// The page: what you were told, and what you can be told on.
//
// The control that turns notifications on stays on /account. That is not an
// oversight: subscribing is proving a device is yours and agreeing to be
// interrupted on it, which is the same category as verifying a phone number,
// and /account is where this product keeps the things you prove. What was
// missing is the other half — a place to see what has actually been sent.
//
// Notifications are the one message this product sends that leaves no copy.
// Mail is in the mailbox, a text is in the record, an answer is in the thread;
// a push disappeared into the handset and, if it never arrived, into nothing at
// all. So "did the thing I set up ever fire" and "what keeps waking me" had no
// answer anywhere. This is the answer.

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/push"
)

// Handler serves /notify.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	who := sess.Account

	sent := History(who, 50)
	devices := Devices(who)

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{
			"reachable": Reachable(who), "devices": devices, "sent": sent,
		})
		return
	}

	var b strings.Builder

	if !push.Configured() {
		b.WriteString(`<div class="card"><h3>Not set up</h3>` +
			`<p class="text-sm text-muted">This instance cannot send notifications: it has no ` +
			`push keys. They are minted on first use and stored, so this usually means the ` +
			`settings store is not writable.</p></div>`)
		app.Respond(w, r, app.Response{Title: "Notifications",
			Description: "Reach yourself when you are not looking at the page", HTML: b.String()})
		return
	}

	b.WriteString(devicesCard(devices))
	b.WriteString(historyCard(sent))

	app.Respond(w, r, app.Response{Title: "Notifications",
		Description: "Reach yourself when you are not looking at the page", HTML: b.String()})
}

// devicesCard is where you can be reached, and the way to change that.
//
// It does not repeat the button. /account owns turning it on — that flow needs
// the browser's permission prompt and the subscription it hands back, and two
// copies of it on two pages is two things to keep working. This says what the
// server knows and points at the one place that can change it.
func devicesCard(devices []push.Subscription) string {
	var b strings.Builder
	b.WriteString(`<div class="card"><h3>Where you can be reached</h3>`)

	if len(devices) == 0 {
		b.WriteString(`<p class="text-sm text-muted">No device is registered, so nothing sent ` +
			`here can arrive. Turn it on at ` + app.TextLink("/account", "/account") +
			` — the browser asks first, on the device you want to be reached on.</p>`)
		b.WriteString(`</div>`)
		return b.String()
	}

	b.WriteString(`<ul class="notify-devices">`)
	for _, d := range devices {
		label := strings.TrimSpace(d.Label)
		if label == "" {
			// Not "Unknown device". The browser is simply not obliged to say
			// what it is, and a row that says so is more useful than a row that
			// implies something went wrong.
			label = "A device that did not say what it was"
		}
		b.WriteString(`<li><span class="notify-device">` + html.EscapeString(label) + `</span>`)
		b.WriteString(`<span class="notify-when">on since ` + html.EscapeString(app.TimeAgo(d.Added)) + `</span>`)
		// The truth about this device, which is not the same as the truth about
		// the account. One phone can be delivering while a dead laptop refuses
		// everything, and an account-wide "on" hides that.
		switch {
		case d.Failed.After(d.Sent) && d.Error != "":
			b.WriteString(`<span class="notify-bad">last try failed: ` +
				html.EscapeString(d.Error) + `</span>`)
		case !d.Sent.IsZero():
			b.WriteString(`<span class="notify-ok">last delivered ` +
				html.EscapeString(app.TimeAgo(d.Sent)) + `</span>`)
		default:
			b.WriteString(`<span class="notify-when">nothing sent here yet</span>`)
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul>`)
	b.WriteString(`<p class="text-sm text-muted">Turn devices on and off, and send a test, at ` +
		app.TextLink("/account", "/account") + `.</p>`)
	b.WriteString(`</div>` + pageCSS)
	return b.String()
}

// historyCard is what has been sent, newest first.
func historyCard(sent []push.Sent) string {
	var b strings.Builder
	b.WriteString(`<div class="card"><h3>What you were told</h3>`)

	if len(sent) == 0 {
		b.WriteString(`<p class="text-sm text-muted">Nothing yet. Mail arriving and a reminder ` +
			`firing send one, and so does an agent that has been asked to tell you something.</p>`)
		b.WriteString(`</div>`)
		return b.String()
	}

	b.WriteString(`<ul class="notify-list">`)
	for _, s := range sent {
		b.WriteString(`<li class="notify-item">`)
		b.WriteString(`<div class="notify-head">`)
		title := html.EscapeString(s.Title)
		if s.URL != "" {
			title = `<a href="` + html.EscapeString(s.URL) + `">` + title + `</a>`
		}
		b.WriteString(`<span class="notify-title">` + title + `</span>`)
		if s.From != "" {
			b.WriteString(`<span class="notify-from">` + html.EscapeString(s.From) + `</span>`)
		}
		b.WriteString(`<span class="notify-when">` + html.EscapeString(app.TimeAgo(s.At)) + `</span>`)
		b.WriteString(`</div>`)
		if s.Body != "" {
			b.WriteString(`<div class="notify-body">` + html.EscapeString(s.Body) + `</div>`)
		}
		// A notification that failed is the whole reason this list exists, so it
		// says so rather than looking identical to one that arrived.
		if !s.OK {
			why := s.Error
			if why == "" {
				why = "it was not delivered"
			}
			b.WriteString(`<div class="notify-bad">Not delivered: ` + html.EscapeString(why) + `</div>`)
		} else {
			// What the device said, which is the half that used to be dark.
			//
			// "Accepted by the push service" is where this stopped, and it
			// cannot distinguish a notification the handset showed from one it
			// never woke up for — the two look identical from the server, and
			// the difference is the whole question. The service worker posts a
			// receipt now. Three states, and each sends you somewhere
			// different: shown, woke-but-could-not, and silence.
			switch {
			case s.Got.IsZero():
				b.WriteString(`<div class="notify-wait">Accepted by the push service. ` +
					`This device has not reported picking it up — either it has not woken ` +
					`yet, or it never will.</div>`)
			case s.Shown:
				b.WriteString(`<div class="notify-got">Shown on your device ` +
					html.EscapeString(app.TimeAgo(s.Got)) + `.</div>`)
			default:
				why := s.Why
				if why == "" {
					why = "it did not say why"
				}
				b.WriteString(`<div class="notify-bad">Your device woke up and could not ` +
					`show it: ` + html.EscapeString(why) + `</div>`)
			}
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul></div>` + pageCSS)
	return b.String()
}

const pageCSS = `<style>
.notify-devices,.notify-list{list-style:none;margin:0;padding:0}
.notify-devices li{display:flex;flex-wrap:wrap;align-items:baseline;gap:10px;padding:8px 0;
  border-bottom:1px solid var(--card-border,#eee)}
.notify-devices li:last-child{border-bottom:0}
.notify-device{font-size:14px;color:var(--text-primary,#111)}
.notify-item{padding:10px 0;border-bottom:1px solid var(--card-border,#eee)}
.notify-item:last-child{border-bottom:0}
.notify-head{display:flex;flex-wrap:wrap;align-items:baseline;gap:10px}
.notify-title{font-size:14px;font-weight:600;color:var(--text-primary,#111)}
.notify-from{font-size:12px;color:#888}
/* Out to the right, the way the time sits on a row in the inbox and on /agents. */
.notify-when{margin-left:auto;flex:none;font-size:12px;color:#aaa;white-space:nowrap}
.notify-body{font-size:13px;color:#666;line-height:1.5;margin-top:2px}
.notify-ok{font-size:12px;color:#888}
.notify-bad{font-size:12px;color:var(--danger,#c33);margin-top:2px}
/* What the device said. Green when it showed, muted while nothing has come
   back — the waiting state is not a failure, it is an unanswered question. */
.notify-got{font-size:12px;color:var(--btn-success,#1a7f37);margin-top:2px}
.notify-wait{font-size:12px;color:var(--text-muted,#888);margin-top:2px}
@media only screen and (max-width:600px){.notify-when{margin-left:0}}
</style>`
