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
	creating := id == "new"
	if id != "" && !creating {
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
		if creating {
			invitees := strings.FieldsFunc(r.PostFormValue("people"), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
			for i, who := range invitees {
				who = strings.ToLower(strings.TrimPrefix(who, "@"))
				person, e := auth.GetAccount(who)
				if e != nil || person == nil || person.Agent || auth.IsBanned(who) {
					app.BadRequest(w, r, "Choose existing people's usernames. You can share an invitation link from the group after creating it.")
					return
				}
				invitees[i] = who
			}
			created, e := group.Start(acc.ID, r.PostFormValue("name"), r.PostFormValue("description"), selected, invitees)
			if e != nil {
				app.BadRequest(w, r, e.Error())
				return
			}
			http.Redirect(w, r, "/chat?id="+group.RoomPrefix+created.ID, http.StatusSeeOther)
			return
		}
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
	b.WriteString(`<div class="page-stack"><div class="form-actions"><a href="/agents">Agents</a><a href="/agent?view=chats">All chats</a></div>`)
	if notice != "" {
		b.WriteString(app.Problem(notice))
	}
	b.WriteString(`<form class="form record-editor" method="get" action="/agent"><input type="hidden" name="view" value="new-chat"><label>Who is this chat for?<select name="group" data-submit-on-change><option value="">Just me</option>`)
	gs, _, err := group.List(acc.ID)
	if err != nil {
		app.Error(w, r, 503, "Groups are temporarily unavailable")
		return
	}
	newSelected := ""
	if creating {
		newSelected = " selected"
	}
	b.WriteString(`<option value="new"` + newSelected + `>New group</option>`)
	for _, item := range gs {
		selected := ""
		if item.ID == id {
			selected = " selected"
		}
		b.WriteString(`<option value="` + html.EscapeString(item.ID) + `"` + selected + `>` + html.EscapeString(item.Name) + `</option>`)
	}
	b.WriteString(`</select></label><div class="form-actions"><noscript><button>Choose</button></noscript><a href="/agent?view=new-chat&amp;group=new">Create a group</a></div></form>`)
	b.WriteString(`<form class="form record-editor" method="post" action="/agent?view=new-chat">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="group" value="` + html.EscapeString(id) + `">`)
	if creating {
		b.WriteString(`<label>Group name<input name="name" required maxlength="100" placeholder="Family"></label><label>Description (optional)<input name="description" maxlength="1000"></label><label>Invite people (optional)<input name="people" placeholder="@alice, @bob" autocomplete="off"></label><p class="muted">Use usernames from this Mu server. People must accept before they can read the chat. You can also share an invitation link from the group.</p>`)
	} else if id != "" {
		b.WriteString(`<p>Shared with ` + html.EscapeString(g.Name) + `.</p><label>Chat name<input name="name" required maxlength="100" value="` + html.EscapeString(r.PostFormValue("name")) + `" placeholder="Weekend plans"></label>`)
	} else {
		b.WriteString(`<p>A private conversation with your assistant.</p>`)
	}
	b.WriteString(`<label>Assistant<select name="agent">`)
	if !g.Encrypted {
		microSelected := ""
		if r.URL.Query().Get("agent") == "" || r.URL.Query().Get("agent") == DefaultPlatformAgent {
			microSelected = " selected"
		}
		b.WriteString(`<option value=""` + microSelected + `>Micro</option>`)
		if id == "" {
			for _, key := range PlatformNames() {
				if key == DefaultPlatformAgent {
					continue
				}
				if a := Platform(key); a != nil {
					selected := ""
					if a.ID == r.URL.Query().Get("agent") {
						selected = " selected"
					}
					b.WriteString(`<option value="` + html.EscapeString(a.ID) + `"` + selected + `>` + html.EscapeString(a.Name) + `</option>`)
				}
			}
		}
		available := Agents(scope)
		if creating {
			available = nil
		}
		for _, a := range available {
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
	b.WriteString(`</select></label>`)
	if id != "" && !g.Encrypted {
		b.WriteString(`<p class="muted">Mention @micro to ask the assistant. Assistant usage uses the group's balance.</p>`)
	}
	b.WriteString(`<div class="form-actions"><button>Start chat</button></div></form></div>`)
	app.Respond(w, r, app.Response{Title: "New chat", HTML: b.String()})
}
