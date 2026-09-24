package home

import (
	"context"
	"html"
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/result"
	"mu/internal/service"
	"mu/internal/thread"
	"mu/service/apps"
	"net/http"
	"net/url"
	"time"
)

// AppsHandler is the personal collection, composed above the app service.
// Its service read is local metadata only: no model or external data fetch.
func AppsHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	auth.SetCSRFCookie(w, r)
	ctx, cancel := context.WithTimeout(service.WithAccount(r.Context(), acc.ID), 5*time.Second)
	defer cancel()
	var collection apps.CollectionResponse
	if err := service.Call(ctx, "apps", "Server.Collection", &apps.CollectionRequest{}, &collection); err != nil {
		app.Error(w, r, 503, "Your apps could not be loaded. Please try again.")
		return
	}
	if r.Method == http.MethodPost {
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		slug := r.FormValue("slug")
		for _, a := range collection.Items {
			if a.Slug != slug {
				continue
			}
			th := thread.Get(acc.ID, a.Source)
			if th == nil {
				th = thread.Open(acc.ID, thread.WebClient, "app:"+slug)
			}
			if th == nil {
				app.Error(w, r, 503, "Could not open the conversation")
				return
			}
			if len(thread.Messages(acc.ID, th.ID, 1)) == 0 {
				thread.Name(acc.ID, th.ID, a.Name)
				thread.Add(thread.Message{Account: acc.ID, Thread: th.ID, Role: thread.RoleAgent, Text: "What would you like to change in this app?", Ref: "app-context:" + slug, Results: []result.Item{{Kind: "app", ID: slug, Title: a.Name, URL: "/apps/" + slug}}})
			}
			if err := thread.Flush(); err != nil {
				app.Error(w, r, 503, "Could not save the conversation")
				return
			}
			target := agent.Path(acc.ID, th.Agent) + "?session=" + url.QueryEscape(th.ID)
			if th.Client != thread.WebClient {
				target = "/inbox?id=" + url.QueryEscape(th.ID)
			}
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
		app.NotFound(w, r, "App not found")
		return
	}
	body := tabs(true) + `<div class="form-actions"><a href="/agent/micro">Ask Micro</a><a href="/apps">Browse apps</a><a href="/work">Work</a></div>`
	if len(collection.Items) == 0 {
		body += `<p>Your saved apps will appear here. Ask Micro to build a small tool you need.</p>`
	}
	pager := app.Paginate(r, len(collection.Items), 25)
	body += `<div class="card-grid">`
	for _, a := range collection.Items[pager.From:pager.To] {
		body += `<article class="record-card"><a class="record-title" href="/apps/` + url.PathEscape(a.Slug) + `">` + html.EscapeString(a.Name) + `</a><p>` + html.EscapeString(a.Description) + `</p><p class="text-muted">Updated ` + app.TimeAgo(a.Updated) + `</p><div class="form-actions"><a class="btn" href="/apps/` + url.PathEscape(a.Slug) + `">Open</a><form method="POST" action="/home/apps">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="slug" value="` + html.EscapeString(a.Slug) + `"><button>Continue with Micro</button></form></div></article>`
	}
	body += `</div>` + pager.Nav("/home/apps")
	app.Respond(w, r, app.Response{Title: "My apps", HTML: body, Data: collection})
}
