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
	body := `<div class="page-stack"><section class="record-card"><div class="metadata-row"><time>` + html.EscapeString(e.Written.Format("2 January 2006 · 15:04 MST")) + `</time></div><p>` + html.EscapeString(e.Text) + `</p><form method="POST" action="/brief" class="form-actions">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="id" value="` + e.ID() + `"><button type="submit">Ask about this brief</button></form><p class="text-muted">The brief and its source material will accompany your questions in a private conversation.</p></section>`

	for _, story := range e.Stories {
		body += `<section class="record-card"><h2>` + html.EscapeString(story.Title) + `</h2><p>` + html.EscapeString(story.Detail) + `</p><div class="form-actions">`
		for _, source := range story.Sources {
			parsed, _ := url.Parse(source)
			label := source
			if parsed != nil {
				label = parsed.Host
			}
			body += `<a href="` + html.EscapeString(source) + `" rel="noopener noreferrer">` + html.EscapeString(label) + `</a>`
		}
		body += `</div></section>`
	}
	if e.Material != "" {
		body += `<details class="disclosure"><summary>All source material</summary>` + string(app.RenderNoImages([]byte(e.Material))) + `</details>`
	}

	body += `</div>`
	app.Respond(w, r, app.Response{Title: "Brief", HTML: body})
}
