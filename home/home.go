package home

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/thread"
	"mu/service/mail"
)

// Handler is the personal overview. Sources remain tools; Home shows what is
// useful to this person and provides a short path back to their conversations.
func Handler(w http.ResponseWriter, r *http.Request) {
	ConsoleHandler(w, r)
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

		return sectionRule("Latest brief") + `<section class="card morning-brief">` + briefDeliveredAt(m.CreatedAt) + `<div class="markdown-content">` + app.RenderString(body) + `</div><div class="form-actions">` + app.ActionLink("/?session="+url.QueryEscape(th.ID), "Continue conversation") + `</div></section>`
	}
	return ""
}

func htmlEsc(s string) string { return html.EscapeString(s) }
func sectionRule(label string) string {
	return `<p class="home-section"><small>` + htmlEsc(label) + `</small></p>`
}

// An absolute delivery date keeps an older brief from masquerading as today's.
func briefDeliveredAt(at time.Time) string {
	if at.IsZero() {
		return `<p class="text-muted">Delivery date unavailable</p>`
	}
	return `<p class="text-muted">Delivered <time datetime="` + at.UTC().Format(time.RFC3339) + `">` + at.UTC().Format("2 January 2006 at 15:04 UTC") + `</time></p>`
}
