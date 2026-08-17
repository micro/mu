package recall

// The page, at /recall.
//
// It was headless, and that broke a pattern with no exceptions in it: every
// other service in the catalogue has a page, service name == route == nav label
// == tool prefix, and a service missing from the sidebar is a capability a
// person cannot find. The reasoning for leaving it out was that the record
// already has a page — the conversation list on /agent — and a second list of
// the same conversations is exactly what was just removed from the agent
// surface for being two names for one thing.
//
// Both are true, and the resolution is that this is not a list. /agent browses:
// here are your recent conversations, open one, carry on talking. This searches:
// somewhere in five thousand messages across five clients, somebody mentioned an
// invoice. Same relationship as a feed and a search box, and the tell is that
// this page shows nothing at all until you ask it something.
//
// The other option was a flag on the Spec meaning "is a service, but hide it".
// That was tried, was called Staple, and deleting it was the fix — see CLAUDE.md.
// A category of half-services is how a catalogue stops being a list of things a
// person can name.

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// Handler serves /recall.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	owner := sess.Account

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	only := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("client")))

	var hits []thread.Hit
	if query != "" {
		hits = thread.Search(owner, query, only, 100)
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"query": query, "results": hits})
		return
	}

	var b strings.Builder
	b.WriteString(`<div style="max-width:760px">`)
	b.WriteString(`<p class="lens-lead">Everything you and your agents have said to each other, ` +
		`wherever you said it — here, by email, on Discord, Telegram or WhatsApp. Search it. ` +
		`Your conversations as a list, to carry one on, are on ` + app.Link("your inbox", "/inbox") + `</p>`)

	b.WriteString(`<form method="GET" action="/recall" class="rc-form">` +
		`<input class="rc-input" type="search" name="q" placeholder="A word or phrase somebody said" ` +
		`value="` + html.EscapeString(query) + `" autofocus>` +
		`<button class="rc-go" type="submit">Search</button></form>`)

	// The clients this account has actually used, so an instance with no Discord
	// bot does not offer to narrow to Discord.
	if query != "" {
		b.WriteString(clientChips(owner, query, only))
	}

	switch {
	case query == "":
		b.WriteString(`<p class="rc-empty">Nothing is shown until you ask for something. ` +
			`This is the transcript — everything anybody said, whether or not it turned out to ` +
			`matter. What you asked to be remembered on purpose is in ` +
			app.Link("Notes", "/notes") + `.</p>`)
	case len(hits) == 0:
		b.WriteString(`<p class="rc-empty">Nothing in your history mentions ` +
			`<strong>` + html.EscapeString(query) + `</strong>.</p>`)
	default:
		b.WriteString(`<p class="rc-count">` + plural(len(hits)) + ` mentioning <strong>` +
			html.EscapeString(query) + `</strong></p><div class="rc-hits">`)
		for _, h := range hits {
			b.WriteString(hitRow(h, query))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>` + pageCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Recall", "Search everything you have said to an agent", b.String(), r)))
}

// hitRow is one match: who said it, when, where, and the way into the rest of
// the conversation. A result nothing follows from is a dead end.
func hitRow(h thread.Hit, query string) string {
	who := "You"
	switch {
	case h.Role == thread.RoleAgent:
		who = "Agent"
	case h.From != "":
		who = h.From
	}
	subject := h.Subject
	if subject == "" {
		subject = "Untitled"
	}
	return `<a class="rc-hit" href="/inbox?session=` + url.QueryEscape(h.Thread) + `">` +
		`<div class="rc-meta">` + html.EscapeString(who) + ` · ` +
		html.EscapeString(app.TimeAgo(h.At)) + ` · ` +
		`<span class="rc-where">` + html.EscapeString(clientName(h.Client)) + `</span></div>` +
		`<div class="rc-text">` + highlight(snippet(h.Text, query), query) + `</div>` +
		`<div class="rc-subject">in “` + html.EscapeString(subject) + `”</div></a>`
}

// clientChips narrows a search to where it was said.
func clientChips(owner, query, active string) string {
	seen := map[string]bool{}
	var present []string
	for _, t := range thread.List(owner, 0) {
		if t.Client == "" || seen[t.Client] {
			continue
		}
		seen[t.Client] = true
		present = append(present, t.Client)
	}
	if len(present) < 2 {
		return ""
	}
	sortStrings(present)

	chip := func(label, client string) string {
		cls := "rc-chip"
		if client == active {
			cls += " on"
		}
		q := url.Values{"q": {query}}
		if client != "" {
			q.Set("client", client)
		}
		return `<a class="` + cls + `" href="/recall?` + q.Encode() + `">` +
			html.EscapeString(label) + `</a>`
	}
	var b strings.Builder
	b.WriteString(`<div class="rc-chips">` + chip("Everywhere", ""))
	for _, c := range present {
		b.WriteString(chip(clientName(c), c))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// highlight marks the term inside an already-escaped snippet.
//
// The snippet is escaped first and the mark inserted after, so a message
// containing markup is inert text and the only tags in the output are these.
func highlight(text, query string) string {
	esc := html.EscapeString(text)
	want := html.EscapeString(query)
	if want == "" {
		return esc
	}
	var b strings.Builder
	rest, lower := esc, strings.ToLower(esc)
	lw := strings.ToLower(want)
	for {
		i := strings.Index(lower, lw)
		if i < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:i])
		b.WriteString(`<mark>` + rest[i:i+len(want)] + `</mark>`)
		rest, lower = rest[i+len(want):], lower[i+len(lw):]
	}
}

// clientName is what a client is called in front of somebody. One not named
// here is shown as it names itself, so a client written tomorrow appears the day
// it is written.
func clientName(client string) string {
	switch client {
	case "web":
		return "Here"
	case "mail":
		return "Email"
	case "discord":
		return "Discord"
	case "telegram":
		return "Telegram"
	case "whatsapp":
		return "WhatsApp"
	case "sms":
		return "SMS"
	case "cli":
		return "CLI"
	case "a2a":
		return "A2A"
	}
	return client
}

func plural(n int) string {
	if n == 1 {
		return "1 message"
	}
	return itoa(n) + " messages"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

const pageCSS = `<style>
.lens-lead{color:#666;font-size:14px;margin:0 0 18px;max-width:640px}
.rc-form{display:flex;gap:8px;margin:0 0 16px}
.rc-input{flex:1;padding:9px 12px;font-size:14px;font-family:inherit;border:1px solid var(--border-color,#ddd);border-radius:6px;background:var(--card-background,#fff);color:var(--text-primary,#111)}
.rc-go{padding:9px 16px;background:#111;color:#fff;border:0;border-radius:6px;font-size:14px;font-family:inherit;cursor:pointer}
.rc-chips{display:flex;flex-wrap:wrap;gap:6px;margin:0 0 16px}
.rc-chip{border:1px solid #eee;border-radius:999px;padding:3px 11px;font-size:12px;color:#666;text-decoration:none}
.rc-chip:hover{border-color:#ccc}
.rc-chip.on{background:var(--text-primary,#111);border-color:var(--text-primary,#111);color:#fff}
.rc-count{font-size:13px;color:#888;margin:0 0 10px}
.rc-hits{display:flex;flex-direction:column;gap:8px}
.rc-hit{display:block;border:1px solid #eee;border-radius:8px;padding:10px 14px;text-decoration:none;color:inherit}
.rc-hit:hover{border-color:#ddd;background:#fcfcfc}
.rc-meta{font-size:12px;color:#999}
.rc-where{border:1px solid #eee;border-radius:999px;padding:1px 8px;font-size:11px}
.rc-text{font-size:14px;line-height:1.55;margin:5px 0 4px}
.rc-text mark{background:#fdf3c3;color:inherit;padding:0 1px}
.rc-subject{font-size:12px;color:#aaa}
.rc-empty{color:#888;font-size:14px}
</style>`
