package events

import (
	"html"
	"mu/internal/app"
	"strings"
	"time"
)

// PreviewLimit bounds the Home overview and its provider reads.
const PreviewLimit = 3

// Preview is a compact, read-only overview of the owner's next events.
func Preview(owner string, external []External) string {
	if owner == "" {
		return ""
	}
	now := time.Now()
	rows := mergedRows(Upcoming(owner), external)
	var b strings.Builder
	b.WriteString(`<div class="compact-list">`)
	count := 0
	for _, row := range rows {
		href := externalURL(row.External)
		title, when, allDay := row.External.Title, row.When, row.External.AllDay
		if row.Event != nil {
			if row.Event.Kind == "brief" || row.When.Before(now) {
				continue
			}
			title = row.Event.Title
			href = eventURL(row.Event.ID)
		}
		label := when.Format("Mon 2 Jan, 15:04")
		stamp := ` data-event-time`
		if allDay {
			label = when.Format("Mon 2 Jan") + ", all day"
			stamp = ""
		}
		b.WriteString(`<a href="` + html.EscapeString(href) + `" class="link compact-list-item"><span>` + html.EscapeString(title) + `</span><small class="text-muted"><time datetime="` + when.Format(time.RFC3339) + `"` + stamp + `>` + html.EscapeString(label) + `</time></small></a>`)
		count++
		if count == PreviewLimit {
			break
		}
	}
	if count == 0 {
		b.WriteString(`<p class="text-muted">No upcoming events to show.</p>`)
	}
	b.WriteString(`</div>`)
	return app.PreviewCard("home-events-card", "Upcoming", "/events", b.String())
}
