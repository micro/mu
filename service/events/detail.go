package events

import (
	"html"
	"net/http"
	"net/url"
	"time"

	"mu/internal/app"
)

func eventURL(id string) string { return "/events?id=" + url.QueryEscape(id) }

// Only navigation URLs are accepted from an external calendar.
func externalURL(e External) string {
	u, err := url.Parse(e.URL)
	if err == nil && u.Scheme == "https" && u.Host != "" {
		return u.String()
	}
	return "/events"
}

func ownedEvent(owner, id string) *Event {
	mu.RLock()
	defer mu.RUnlock()
	e := events[id]
	if e == nil || e.Owner != owner {
		return nil
	}
	copy := *e
	return &copy
}

func detailHandler(w http.ResponseWriter, r *http.Request, owner, id string) {
	e := ownedEvent(owner, id)
	if e == nil {
		http.NotFound(w, r)
		return
	}
	body := `<div class="page-col page-stack"><article class="card page-stack"><p><time datetime="` + e.When.Format(time.RFC3339) + `" data-event-time>` + html.EscapeString(e.When.Format("Mon 2 Jan, 15:04")) + `</time></p>`
	if e.Note != "" {
		body += `<div style="white-space:pre-wrap">` + html.EscapeString(e.Note) + `</div>`
	}
	if e.Repeat != "" {
		body += `<p>Repeats: ` + html.EscapeString(e.Repeat) + `</p>`
	}
	body += `<a class="link" href="` + html.EscapeString(GoogleCalendarURL(e.Title, e.When, e.Note)) + `" target="_blank" rel="noopener">Add to calendar</a></article><a class="link" href="/events">Back to events →</a></div>`
	body += `<script>document.querySelectorAll('time[data-event-time]').forEach(function(el){var d=new Date(el.dateTime);if(!isNaN(d.getTime()))el.textContent=d.toLocaleString(undefined,{weekday:'short',day:'numeric',month:'short',hour:'2-digit',minute:'2-digit'});});</script>`
	app.Respond(w, r, app.Response{Title: e.Title, HTML: body})
}
