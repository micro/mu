package agent

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/group"
)

func newChatHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != "GET" && r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	id := r.URL.Query().Get("group")
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if r.ParseForm() != nil || !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid form")
			return
		}
		if err := auth.CheckPostRate(acc.ID); err != nil {
			http.Error(w, err.Error(), 429)
			return
		}
		id = r.PostFormValue("group")
	}
	scope := acc.ID
	var g group.Group
	if id != "" {
		g, err = group.Read(id, acc.ID)
		if err != nil {
			app.NotFound(w, r, "Group not found")
			return
		}
		scope = group.Scope(id)
	}
	notice := ""
	if r.Method == "POST" {
		selected := r.PostFormValue("agent")
		if id == "" {
			if selected != "" && For(acc.ID, selected) == nil && Platform(selected) == nil {
				app.BadRequest(w, r, "Choose an assistant")
				return
			}
			http.Redirect(w, r, Path(acc.ID, selected)+"?new=1", 303)
			return
		}
		if selected != "" && selected != "none" && For(scope, selected) == nil {
			app.BadRequest(w, r, "Choose a group assistant")
			return
		}
		c, e := group.SaveConversation(id, acc.ID, "", r.PostFormValue("name"), selected)
		if e == nil {
			http.Redirect(w, r, "/chat?id="+url.QueryEscape(c.ID), 303)
			return
		}
		notice = e.Error()
	}
	auth.SetCSRFCookie(w, r)
	var b strings.Builder
	b.WriteString(`<div class="page-stack">`)
	if notice != "" {
		b.WriteString(app.Problem(notice))
	}
	b.WriteString(`<form class="form record-editor" method="get" action="/agent"><input type="hidden" name="view" value="new-chat"><label>Who is this chat for?<select name="group" data-submit-on-change><option value="">Just me</option>`)
	gs, _, err := group.List(acc.ID)
	if err != nil {
		app.Error(w, r, 503, "Groups are temporarily unavailable")
		return
	}
	for _, item := range gs {
		selected := ""
		if item.ID == id {
			selected = " selected"
		}
		b.WriteString(`<option value="` + html.EscapeString(item.ID) + `"` + selected + `>` + html.EscapeString(item.Name) + `</option>`)
	}
	b.WriteString(`</select></label><div class="form-actions"><noscript><button>Choose</button></noscript><a href="/groups?new=1">Create a group</a></div></form>`)
	b.WriteString(`<form class="form record-editor" method="post" action="/agent?view=new-chat">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="group" value="` + html.EscapeString(id) + `">`)
	if id != "" {
		b.WriteString(`<p>Shared with ` + html.EscapeString(g.Name) + `.</p><label>Chat name<input name="name" required maxlength="100" value="` + html.EscapeString(r.PostFormValue("name")) + `" placeholder="Weekend plans"></label>`)
	} else {
		b.WriteString(`<p>A private conversation with your assistant.</p>`)
	}
	b.WriteString(`<label>Assistant<select name="agent">`)
	if !g.Encrypted {
		b.WriteString(`<option value="">Micro</option>`)
		if id == "" {
			for _, key := range PlatformNames() {
				if key == DefaultPlatformAgent {
					continue
				}
				if a := Platform(key); a != nil {
					b.WriteString(`<option value="` + html.EscapeString(a.ID) + `">` + html.EscapeString(a.Name) + `</option>`)
				}
			}
		}
		for _, a := range Agents(scope) {
			selected := ""
			if a.ID == r.URL.Query().Get("agent") {
				selected = " selected"
			}
			b.WriteString(`<option value="` + html.EscapeString(a.ID) + `"` + selected + `>` + html.EscapeString(a.Name) + `</option>`)
		}
	}
	if id != "" {
		b.WriteString(`<option value="none">People only</option>`)
	}
	b.WriteString(`</select></label><div class="form-actions"><button>Start chat</button></div></form></div>`)
	app.Respond(w, r, app.Response{Title: "New chat", HTML: b.String()})
}
