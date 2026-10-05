package agent

import (
	"html"
	"mu/agent/brief"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/url"
)

// BriefHandler serves cached material and attaches it to a private conversation.
// Opening a brief or its conversation never calls a model.
func BriefHandler(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	if acc == nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	var e brief.Entry
	var ok bool
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil || !auth.StrictCSRF(r) {
			http.Error(w, "Invalid request", http.StatusForbidden)
			return
		}
		e, ok = brief.Get(r.PostForm.Get("id"))
	} else if id := r.URL.Query().Get("id"); id != "" {
		e, ok = brief.Get(id)
	} else {
		e, ok = brief.Latest()
	}
	if !ok {
		app.NotFound(w, r, "Brief not available")
		return
	}
	if r.Method == http.MethodPost {
		if err := brief.Pin(e); err != nil {
			http.Error(w, "Could not save brief", http.StatusInternalServerError)
			return
		}
		t := thread.Open(acc.ID, thread.WebClient, "brief:"+e.ID())
		thread.SetAttachment(acc.ID, t.ID, "brief:"+e.ID())
		thread.Name(acc.ID, t.ID, "Brief · "+e.Written.Format("2 January"))
		if err := thread.Flush(); err != nil {
			http.Error(w, "Could not save conversation", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, Path(acc.ID, "")+"?session="+url.QueryEscape(t.ID), http.StatusSeeOther)
		return
	}
	auth.SetCSRFCookie(w, r)
	if app.WantsJSON(r) {
		app.RespondJSON(w, e)
		return
	}
	body := `<article class="editorial-page"><div class="metadata-row"><time>` + html.EscapeString(e.Written.Format("2 January 2006 · 15:04 MST")) + `</time></div><div class="reader-content"><p>` + html.EscapeString(e.Text) + `</p></div><form method="POST" action="/brief" class="form-actions">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="id" value="` + e.ID() + `"><button type="submit">Ask about this brief</button></form>`

	// Keep references secondary; the full material remains attached to questions.
	sources := ""
	seen := make(map[string]bool)
	for _, story := range e.Stories {
		for _, source := range story.Sources {
			parsed, err := url.Parse(source)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || seen[source] {
				continue
			}
			seen[source] = true
			sources += `<li><a href="` + html.EscapeString(source) + `" rel="noopener noreferrer">` + html.EscapeString(story.Title) + `</a> <span class="text-muted text-sm">` + html.EscapeString(parsed.Hostname()) + `</span></li>`
		}
	}
	if sources != "" {
		body += `<details class="disclosure"><summary>Sources</summary><ul class="reference-list">` + sources + `</ul></details>`
	}
	body += `</article>`
	app.Respond(w, r, app.Response{Title: "Brief", HTML: body})
}
