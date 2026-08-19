package home

// Somewhere to say that something is broken.
//
// There was nowhere. The Discord invite lived in the README on GitHub, and
// support@ was a reserved username with no mailbox behind it — mail to it was
// refused at RCPT and bounced. So an account that could not top up, or paid and
// got nothing, had no way to report it, and the operator found out by being
// told in person. Which is how three separate silent payment failures survived
// to be found by the operator paying for his own product.
//
// A page rather than a mailto in the footer, because the answer depends on what
// went wrong: money is one route, a bug is another, and an instance with no mail
// domain has neither and should say so rather than offering an address that
// cannot receive.

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/settings"
	"mu/service/mail"
)

// SupportHandler serves /support.
func SupportHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	b.WriteString(`<div style="max-width:680px">`)
	b.WriteString(`<div class="card"><h2>Support</h2>`)
	b.WriteString(`<p style="color:#666;font-size:15px;margin:0">Something not working, a payment that did not land, or a question the docs do not answer.</p>`)
	b.WriteString(`</div>`)

	addr := mail.SupportAddress()
	if addr != "" {
		b.WriteString(`<div class="card" style="margin-top:16px"><h3>Email</h3>`)
		b.WriteString(`<p style="font-size:15px;margin:0 0 6px"><a href="mailto:` +
			html.EscapeString(addr) + `"><code>` + html.EscapeString(addr) + `</code></a></p>`)
		b.WriteString(`<p style="font-size:14px;color:#666;margin:0">Reaches this instance's operator. ` +
			`If it is about money, say what you tried to buy and roughly when — the payment can be ` +
			`found from that.</p>`)
		b.WriteString(`</div>`)
	}

	// The invite is a setting because it is per-instance: somebody self-hosting
	// has their own community or none, and pointing their users at ours would be
	// sending them somewhere nobody can help them.
	if invite := strings.TrimSpace(settings.Get("SUPPORT_CHAT")); invite != "" {
		b.WriteString(`<div class="card" style="margin-top:16px"><h3>Chat</h3>`)
		b.WriteString(`<p style="font-size:15px;margin:0"><a href="` + html.EscapeString(invite) + `" rel="noopener">` +
			html.EscapeString(invite) + `</a></p>`)
		b.WriteString(`</div>`)
	}

	b.WriteString(`<div class="card" style="margin-top:16px"><h3>Bugs</h3>`)
	b.WriteString(`<p style="font-size:15px;margin:0 0 6px"><a href="https://github.com/micro/mu/issues" rel="noopener">github.com/micro/mu/issues</a></p>`)
	b.WriteString(`<p style="font-size:14px;color:#666;margin:0">The code is open. If you can describe what you did and what happened, that is a bug report.</p>`)
	b.WriteString(`</div>`)

	if addr == "" {
		b.WriteString(`<div class="card" style="margin-top:16px">`)
		b.WriteString(`<p style="font-size:14px;color:#666;margin:0">This instance has no mail domain configured, ` +
			`so there is no support address on it. An operator sets <code>MAIL_DOMAIN</code> to change that.</p>`)
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Support", Description: "How to reach this instance's operator", HTML: b.String()})
}
