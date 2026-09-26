package groups

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/group"
	"mu/internal/service"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	id := r.URL.Query().Get("id")
	notice := ""
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", 400)
			return
		}
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		if err := auth.CheckPostRate(acc.ID); err != nil {
			http.Error(w, err.Error(), 429)
			return
		}
		id = r.PostFormValue("id")
		ctx := service.WithAccount(r.Context(), acc.ID)
		if r.PostFormValue("action") == "create" {
			var rsp GroupResponse
			err = (Server{}).Create(ctx, &CreateRequest{Name: r.PostFormValue("name"), Encrypted: r.PostFormValue("encrypted") == "true"}, &rsp)
			if err == nil {
				id = rsp.Group.ID
			}
		} else {
			var rsp UpdateResponse
			err = (Server{}).Update(ctx, &UpdateRequest{ID: id, Action: r.PostFormValue("action"), Account: r.PostFormValue("account"), Value: r.PostFormValue("value")}, &rsp)
			if err == nil && !group.Member(id, acc.ID) {
				id = ""
			}
		}
		if err == nil {
			dest := "/groups"
			if id != "" {
				dest += "?id=" + id
			}
			http.Redirect(w, r, dest, http.StatusSeeOther)
			return
		}
		notice = err.Error()
		if app.WantsJSON(r) {
			app.RespondError(w, http.StatusBadRequest, notice)
			return
		}
	} else if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	auth.SetCSRFCookie(w, r)
	csrf := auth.CSRFToken(r)
	var b strings.Builder
	b.WriteString(`<div class="page-stack">`)
	if notice != "" {
		b.WriteString(`<p role="alert" class="notice bad">` + html.EscapeString(notice) + `</p>`)
	}
	title := "Groups"
	if id != "" && group.Member(id, acc.ID) {
		g, e := group.Read(id, acc.ID)
		if e != nil {
			app.NotFound(w, r, "Group not found")
			return
		}
		if app.WantsJSON(r) {
			app.RespondJSON(w, GroupResponse{Group: g, Chat: "/chat?id=" + group.RoomPrefix + g.ID})
			return
		}
		title = g.Name
		b.WriteString(`<div class="form-actions"><a href="/groups">All groups</a><a class="btn" href="/chat?id=` + group.RoomPrefix + g.ID + `">Open chat</a></div>`)
		b.WriteString(`<p class="text-muted">A private space for this group. Members can read the shared chat history. Your personal notes and conversations stay private.</p>`)
		b.WriteString(`<section class="record-card"><h2>Members</h2><div class="collection-list">`)
		names := make([]string, 0, len(g.Members))
		for a := range g.Members {
			names = append(names, a)
		}
		sort.Strings(names)
		role := g.Members[acc.ID]
		for _, a := range names {
			b.WriteString(`<div class="collection-item"><div class="metadata-row"><a href="/@` + html.EscapeString(a) + `">@` + html.EscapeString(a) + `</a><span>` + g.Members[a] + `</span></div>`)
			if a != acc.ID && role == "owner" {
				options := ""
				for _, choice := range []struct{ value, label string }{{"member", "Member"}, {"admin", "Admin"}, {"owner", "Transfer ownership"}} {
					selected := ""
					if g.Members[a] == choice.value {
						selected = " selected"
					}
					options += `<option value="` + choice.value + `"` + selected + `>` + choice.label + `</option>`
				}
				b.WriteString(`<details><summary>Manage member</summary>` + form(csrf, g.ID, "role", a, `<label class="field-label">Role<select name="value">`+options+`</select></label><button>Save role</button>`) + `</details>`)
			}
			if a != g.Owner && (a == acc.ID || role == "owner" || (role == "admin" && g.Members[a] != "admin")) {
				label := "Remove"
				if a == acc.ID {
					label = "Leave group"
				}
				b.WriteString(form(csrf, g.ID, "remove", a, `<button class="btn-secondary">`+label+`</button>`))
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div></section>`)
		if role == "owner" || role == "admin" {
			b.WriteString(`<section class="record-card"><h2>Invite someone</h2><p>Use their Micro username. They must accept before they can read the group. Invitations expire after seven days.</p>`)
			b.WriteString(form(csrf, g.ID, "invite", "", `<label class="field-label">Username<input name="account" placeholder="username" required maxlength="100"></label><button>Invite</button>`))
			for _, inv := range g.Invitations {
				if time.Now().Before(inv.Expires) {
					b.WriteString(form(csrf, g.ID, "remove", inv.Account, `<span>@`+html.EscapeString(inv.Account)+` · Invited</span><button class="btn-secondary">Cancel invitation</button>`))
				}
			}
			b.WriteString(`</section><details class="record-card"><summary>Group settings</summary>`)
			b.WriteString(form(csrf, g.ID, "rename", "", `<label class="field-label">Group name<input name="value" value="`+html.EscapeString(g.Name)+`" required maxlength="100"></label><button>Rename</button>`))
			if role == "owner" {
				b.WriteString(`<p>Deleting the group closes its chat for everyone.</p>` + form(csrf, g.ID, "delete", "", `<button class="btn-secondary">Delete group</button>`))
			}
			b.WriteString(`</details>`)
		}
	} else {
		if id != "" && notice == "" {
			app.NotFound(w, r, "Group not found")
			return
		}
		gs, pending, e := group.List(acc.ID)
		if e != nil {
			http.Error(w, "Groups are temporarily unavailable", 503)
			return
		}
		if app.WantsJSON(r) {
			app.RespondJSON(w, ListResponse{Groups: gs, Invitations: pending})
			return
		}
		if len(pending) > 0 {
			b.WriteString(`<section class="record-card"><h2>Invitations</h2>`)
			for _, inv := range pending {
				b.WriteString(`<div class="record-card"><h3>` + html.EscapeString(inv.Name) + `</h3><p>Invited by @` + html.EscapeString(inv.InvitedBy) + `</p><div class="form-actions">`)
				b.WriteString(form(csrf, inv.ID, "accept", "", `<button>Accept</button>`) + form(csrf, inv.ID, "decline", "", `<button class="btn-secondary">Decline</button>`) + `</div></div>`)
			}
			b.WriteString(`</section>`)
		}
		b.WriteString(`<section class="record-card"><h2>Your groups</h2><div class="collection-list">`)
		for _, g := range gs {
			fmt.Fprintf(&b, `<a class="collection-item" href="/groups?id=%s"><span class="collection-title">%s</span><span class="collection-preview">%d members · %s</span></a>`, g.ID, html.EscapeString(g.Name), len(g.Members), g.Members[acc.ID])
		}
		if len(gs) == 0 {
			b.WriteString(`<p>No groups yet. Create one for your family, friends or community.</p>`)
		}
		b.WriteString(`</div></section><section class="record-card"><h2>New group</h2>` + form(csrf, "", "create", "", `<label class="field-label">Group name<input name="name" required maxlength="100" placeholder="Family"></label><label class="field-label">Chat privacy<select name="encrypted"><option value="false">Web and XMPP chat</option><option value="true">End-to-end encrypted · XMPP clients only</option></select></label><p class="text-muted">Web chat uses an encrypted connection; Micro stores its messages. End-to-end encrypted groups require an XMPP client such as Conversations.</p><button>Create group</button>`) + `</section>`)
	}
	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: title, Description: "Private groups and shared conversations", HTML: b.String()})
}

func form(csrf, id, action, account, body string) string {
	fields := app.CSRFField(csrf) + `<input type="hidden" name="id" value="` + html.EscapeString(id) + `"><input type="hidden" name="action" value="` + action + `">`
	if account != "" {
		fields += `<input type="hidden" name="account" value="` + html.EscapeString(account) + `">`
	}
	return `<form method="POST" action="/groups" class="form-inline">` + fields + body + `</form>`
}
