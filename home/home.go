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

// Handler keeps the old Home route on the shared command surface.
func Handler(w http.ResponseWriter, r *http.Request) {
	ConsoleHandler(w, r)
}

func deliveredBrief(owner string) string {
	body, id, at := latestBrief(owner)
	if id == "" {
		return ""
	}
	return sectionRule("Latest brief") + `<section class="card morning-brief">` + briefDeliveredAt(at) + `<div class="markdown-content">` + app.RenderString(body) + `</div><div class="form-actions">` + app.ActionLink("/?session="+url.QueryEscape(id), "Continue conversation") + `</div></section>`
}

func latestBrief(owner string) (string, string, time.Time) {
	for _, m := range mail.ListMessages(owner, 100) {
		if m.Tag != "brief" {
			continue
		}
		th := thread.ByRef(owner, m.MessageID)
		if th == nil {
			continue
		}
		body := strings.SplitN(m.Body, "\n\n---\n", 2)[0]

		return body, th.ID, m.CreatedAt
	}
	return "", "", time.Time{}
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
