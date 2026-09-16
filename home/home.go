package home

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/agent"
	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/mail"
)

// Handler is the personal overview. Sources remain tools; Home shows what is
// useful to this person and provides a short path back to their conversations.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.FormValue("action") == "status" {
		statusHandler(w, r)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	auth.SetCSRFCookie(w, r)
	var b strings.Builder
	b.WriteString(`<div class="assistant-overview"><p class="overview-date">` + html.EscapeString(time.Now().Format("Monday, 2 January 2006")) + `</p><div class="overview-assistant">`)
	b.WriteString(app.ChatComponent(app.ChatConfig{
		Ask: true, ServerOwned: true, Location: true,
		SelectionScope: acc.ID + ":/home", StorageNS: "home-" + acc.ID,
		TranscriptPath: "/agent/" + agent.DefaultSlug,
		Placeholder:    "What do you need?", AgentName: agent.DefaultName(),
	}))
	b.WriteString(`</div><div class="overview-details">`)
	if brief := deliveredBrief(acc.ID); brief != "" {
		b.WriteString(brief)
	} else {
		b.WriteString(briefHTML(acc.ID))
	}
	if recent := inbox.Preview(acc.ID); recent != "" {
		b.WriteString(sectionRule("Continue a conversation") + recent)
	} else {
		b.WriteString(`<p class="text-muted">Your conversations will appear here, so you can pick up where you left off.</p>`)
	}
	b.WriteString(`</div></div>`)
	// Old shared links still seed the same composer; remove the prompt from history.
	prefill := r.URL.Query().Get("q")
	if prefill == "" {
		prefill = r.URL.Query().Get("prompt")
	}
	if prefill != "" {
		b.WriteString(`<script>(function(){if(window.muChatAsk){window.muChatAsk(` + app.JSString(prefill) + `);}history.replaceState(window.history.state,'','/home');})();</script>`)
	}
	app.Respond(w, r, app.Response{Title: "Home", Description: "Your personal assistant", HTML: b.String(), BodyClass: ` class="page-home"`})
}

func deliveredBrief(owner string) string {
	for _, m := range mail.ListMessages(owner, 100) {
		if m.Tag != "brief" {
			continue
		}
		th := thread.ByRef(owner, m.MessageID)
		if th == nil {
			continue
		}
		body := strings.SplitN(m.Body, "\n\n---\n", 2)[0]
		runes := []rune(body)
		if len(runes) > 1200 {
			body = string(runes[:1200]) + "…"
		}
		return sectionRule("Your brief") + `<section class="card morning-brief"><div class="markdown-content">` + app.RenderString(body) + `</div><div class="form-actions">` + app.ActionLink("/inbox?id="+url.QueryEscape(th.ID), "Continue conversation") + `</div></section>`
	}
	return ""
}

func htmlEsc(s string) string { return html.EscapeString(s) }
func sectionRule(label string) string {
	return `<p class="home-section"><small>` + htmlEsc(label) + `</small></p>`
}
