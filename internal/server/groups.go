package server

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/group"
	"mu/internal/origin"
	"mu/internal/quota"
	"mu/service/chat"
	"mu/service/events"
	"mu/service/groups"
	"mu/service/mail"
	"mu/x402/billing"
)

func groupsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("join") != "" {
		groupJoinHandler(w, r)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	id := r.URL.Query().Get("id")
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 32768)
		if r.ParseForm() != nil || !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid form")
			return
		}
		id = r.PostFormValue("id")
	}
	if r.Method == "POST" {
		switch r.PostFormValue("action") {
		case "create", "accept", "decline", "invite", "remove", "role", "rename", "delete":
			groups.Handler(w, r)
			return
		}
	}
	tab := r.URL.Query().Get("tab")
	if id == "" || (tab == "roles" && r.Method == "GET") {
		groups.Handler(w, r)
		return
	}
	g, err := group.Read(id, acc.ID)
	if err != nil {
		app.NotFound(w, r, "Group not found")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	scope := group.Scope(id)
	base := "/groups?id=" + id
	notice, link := "", ""
	if r.Method == "POST" {
		if err := auth.CheckPostRate(acc.ID); err != nil {
			http.Error(w, err.Error(), 429)
			return
		}
		action := r.PostFormValue("action")
		admin := group.Admin(id, acc.ID)
		switch action {
		case "conversation":
			selected := r.PostFormValue("agent")
			if selected != "" && selected != "none" && agent.For(scope, selected) == nil {
				err = fmt.Errorf("choose a group assistant")
				break
			}
			var c group.Conversation
			c, err = group.SaveConversation(id, acc.ID, r.PostFormValue("room"), r.PostFormValue("name"), selected)
			if err == nil {
				http.Redirect(w, r, "/chat?id="+c.ID, 303)
				return
			}
		case "agent":
			if !admin || g.Encrypted {
				err = fmt.Errorf("only admins of unencrypted groups can configure assistants")
				break
			}
			if selected := r.PostFormValue("agent_id"); selected != "" {
				_, err = agent.UpdateAgent(scope, selected, r.PostFormValue("name"), r.PostFormValue("prompt"), "", agent.GroupTools())
			} else {
				_, _, err = agent.CreateAgent(scope, r.PostFormValue("name"), agent.Hosted, r.PostFormValue("prompt"), "", agent.GroupTools(), false)
			}
			tab = "agents"
		case "event":
			if !admin {
				err = fmt.Errorf("only group admins can schedule events")
				break
			}
			zone := r.PostFormValue("zone")
			loc, e := time.LoadLocation(zone)
			if e != nil {
				err = fmt.Errorf("choose a valid timezone")
				break
			}
			when, e := time.ParseInLocation("2006-01-02T15:04", r.PostFormValue("when"), loc)
			if e != nil {
				err = e
				break
			}
			_, err = events.CreateStanding(scope, r.PostFormValue("title"), when, r.PostFormValue("note"), 30, r.PostFormValue("repeat"), "", zone)
			tab = "events"
		case "cancel_event":
			if !admin {
				err = fmt.Errorf("only group admins can cancel events")
				break
			}
			err = events.Remove(scope, r.PostFormValue("event"))
			tab = "events"
		case "brief":
			if !admin || g.Encrypted {
				err = fmt.Errorf("only admins of unencrypted groups can schedule briefs")
				break
			}
			err = agent.ConfigureGroupBrief(id, r.PostFormValue("time"), r.PostFormValue("zone"))
			tab = "events"
		case "notifications":
			err = group.Mute(id, acc.ID, r.PostFormValue("muted") == "true")
			tab = "members"
		case "end_plan":
			if !admin {
				err = fmt.Errorf("only group admins can end billing")
				break
			}
			err = billing.EndSubscription(r.Context(), scope)
			tab = "budget"
		case "budget":
			n, e := strconv.Atoi(r.PostFormValue("limit"))
			if e != nil {
				err = fmt.Errorf("enter a credit limit")
				break
			}
			err = group.Configure(id, acc.ID, n, r.PostFormValue("members_ai") == "true")
			tab = "budget"
		case "fund":
			if !admin {
				err = fmt.Errorf("only group admins can fund this balance")
				break
			}
			n, e := strconv.Atoi(r.PostFormValue("amount"))
			if e != nil || n <= 0 {
				err = fmt.Errorf("enter a positive credit amount")
				break
			}
			err = billing.TransferCredits(acc.ID, scope, n)
			tab = "budget"
		case "link", "revoke_link", "email_invite":
			if !admin {
				err = fmt.Errorf("only group admins can invite people")
				break
			}
			var token string
			token, err = group.InvitationLink(id, acc.ID, action == "revoke_link")
			if err == nil && token != "" {
				link = origin.URL(r) + "/join#" + token
				if action == "email_invite" {
					_, err = mail.SendOut(acc.ID, acc.Name, r.PostFormValue("email"), "Join "+g.Name+" on Micro", acc.Name+" invited you to "+g.Name+". Join here: "+link+"\nMembers share the group's conversation history. No personal subscription is needed.", "", "")
					if err == nil {
						notice = "Invitation sent."
					}
				}
			}
			tab = "members"
		default:
			groups.Handler(w, r)
			return
		}
		if err != nil {
			notice = err.Error()
		} else if link == "" && notice == "" {
			http.Redirect(w, r, base+"&tab="+url.QueryEscape(tab), 303)
			return
		}
	} else if r.Method != "GET" {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	csrf := auth.CSRFToken(r)
	form := func(action, body string) string {
		return `<form class="form record-editor" method="POST" action="` + base + `">` + app.CSRFField(csrf) + `<input type="hidden" name="id" value="` + id + `"><input type="hidden" name="action" value="` + action + `">` + body + `</form>`
	}
	var b strings.Builder
	b.WriteString(`<div class="page-stack">`)
	b.WriteString(app.ViewNavigation("Group", tab, []app.ViewLink{{Label: "Chats", URL: base, Key: ""}, {Label: "Events", URL: base + "&tab=events", Key: "events"}, {Label: "Agents", URL: base + "&tab=agents", Key: "agents"}, {Label: "Members", URL: base + "&tab=members", Key: "members"}, {Label: "Budget", URL: base + "&tab=budget", Key: "budget"}, {Label: "Settings", URL: base + "&tab=settings", Key: "settings"}}, false))
	if notice != "" {
		b.WriteString(`<p class="notice" role="status">` + html.EscapeString(notice) + `</p>`)
	}
	admin := group.Admin(id, acc.ID)
	switch tab {
	case "conversation":
		_, c, ok := group.ConversationFor(r.URL.Query().Get("room"))
		if !ok || group.RoomGroup(c.ID) != id {
			app.NotFound(w, r, "Conversation not found")
			return
		}
		if admin {
			picker := groupAgentPicker(scope, g.Encrypted)
			picker = strings.Replace(picker, `value="`+html.EscapeString(c.Agent)+`"`, `value="`+html.EscapeString(c.Agent)+`" selected`, 1)
			b.WriteString(form("conversation", `<input type="hidden" name="room" value="`+c.ID+`"><label>Name<input name="name" value="`+html.EscapeString(c.Name)+`" required></label>`+picker+`<button>Save chat</button>`))
		} else {
			b.WriteString(`<p>Ask a group admin to change this chat's assistant.</p>`)
		}
	case "events":
		b.WriteString(`<div class="collection-list">`)
		for _, e := range events.List(scope) {
			b.WriteString(`<div class="collection-item"><strong>` + html.EscapeString(e.Title) + `</strong><span>` + html.EscapeString(e.LocalTime().Format("Mon 2 Jan, 15:04 MST")) + `</span>`)
			if admin {
				b.WriteString(form("cancel_event", `<input type="hidden" name="event" value="`+e.ID+`"><button class="btn-secondary">Cancel</button>`))
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
		if admin {
			b.WriteString(`<details class="record-card"><summary>New event</summary>` + form("event", `<label>Title<input name="title" required maxlength="200"></label><label>When<input type="datetime-local" name="when" required></label>`+groupZoneField()+`<label>Notes<textarea name="note" maxlength="4000"></textarea></label><label>Repeat<select name="repeat"><option value="">Once</option><option>daily</option><option>weekly</option><option>monthly</option></select></label><button>Create event</button>`) + `</details>`)
			if !g.Encrypted {
				b.WriteString(`<details class="record-card"><summary>Daily group brief</summary><p>A short update on your shared events and chats, delivered once to the group. Requires Starter or Pro.</p>` + form("brief", `<label>Time<input name="time" type="time" required value="08:00"></label>`+groupZoneField()+`<button>Schedule brief</button>`) + `</details>`)
			}
		}
	case "agents":
		b.WriteString(`<p>Micro is available by default. Group assistants use shared context; personal accounts stay private.</p><div class="collection-list"><div class="collection-item"><strong>Micro</strong><span>Default assistant</span></div>`)
		for _, a := range agent.Agents(scope) {
			b.WriteString(`<div class="collection-item"><strong>` + html.EscapeString(a.Name) + `</strong>`)
			if admin {
				b.WriteString(`<details><summary>Edit assistant</summary>` + form("agent", `<input type="hidden" name="agent_id" value="`+a.ID+`"><label>Name<input name="name" value="`+html.EscapeString(a.Name)+`" required></label><label>Instructions<textarea name="prompt" required>`+html.EscapeString(a.Prompt)+`</textarea></label><button>Save assistant</button>`) + `</details>`)
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
		if admin && !g.Encrypted {
			b.WriteString(`<details class="record-card"><summary>New assistant</summary>` + form("agent", `<label>Name<input name="name" maxlength="60" required></label><label>Instructions<textarea name="prompt" maxlength="8000" required></textarea></label><button>Create assistant</button>`) + `</details>`)
		}
	case "members":
		checked := ""
		if g.Muted[acc.ID] {
			checked = " checked"
		}
		b.WriteString(form("notifications", `<label class="check-label"><input type="checkbox" name="muted" value="true"`+checked+`> Mute group notifications</label><button>Save preference</button>`))
		b.WriteString(`<p>` + strconv.Itoa(len(g.Members)) + ` members · New members can read earlier chats.</p><a href="` + base + `&tab=roles">Manage members and roles</a>`)
		names := []string{}
		for name := range g.Members {
			names = append(names, name)
		}
		sort.Strings(names)
		b.WriteString(`<div class="collection-list">`)
		for _, name := range names {
			b.WriteString(`<div class="collection-item">@` + html.EscapeString(name) + ` · ` + g.Members[name] + `</div>`)
		}
		b.WriteString(`</div>`)
		if admin {
			b.WriteString(`<p>Invitation links expire after seven days. Revoke links to cancel all outstanding link invitations.</p>`)
			if link != "" {
				b.WriteString(`<label>Invitation link<input readonly value="` + html.EscapeString(link) + `"></label>`)
			}
			b.WriteString(form("link", `<button>Create invitation link</button>`) + form("email_invite", `<label>Email<input type="email" name="email" required></label><button>Email invitation</button>`) + form("revoke_link", `<button class="btn-secondary">Revoke invitation links</button>`))
		}
	case "settings":
		if admin {
			b.WriteString(`<section class="record-card"><h2>Group name</h2>` + form("rename", `<label>Group name<input name="value" value="`+html.EscapeString(g.Name)+`" required maxlength="100"></label><button>Save name</button>`) + `</section>`)
		} else {
			b.WriteString(`<p>Only group admins can change group settings.</p>`)
		}

		if g.Owner == acc.ID {
			b.WriteString(`<section class="record-card"><h2>Delete group</h2><p>Closes this group and its chats for everyone.</p>` + strings.Replace(form("delete", `<button class="btn-secondary">Delete group</button>`), `<form `, `<form data-confirm="Delete this group for everyone? This cannot be undone." `, 1) + `</section>`)
		}
	case "budget":
		b.WriteString(fmt.Sprintf(`<p><strong>%d credits available</strong></p><p>%d of %d credits used this calendar month (UTC).</p><p>AI usage is shared. Members do not need their own subscription. Chat continues when credits run out.</p>`, quota.Available(scope), billing.GroupSpent(scope), g.SpendLimit))
		if admin {
			checked := ""
			if g.MembersUseAI {
				checked = " checked"
			}
			b.WriteString(form("budget", fmt.Sprintf(`<label>Monthly spending limit (credits)<input type="number" min="0" max="1000000" name="limit" value="%d" required></label><label class="check-label"><input type="checkbox" name="members_ai" value="true"%s> Members can ask assistants</label><button>Save budget</button>`, g.SpendLimit, checked)))
			b.WriteString(form("fund", `<label>Transfer prepaid credits from your balance<input type="number" min="1" name="amount" required></label><button>Fund group</button>`) + `<a href="/account/topup">Top up your prepaid balance</a>`)
			b.WriteString(billing.GroupSubscriptionSummary(r, g))
			if time.Now().Before(g.BillingUntil) {
				b.WriteString(`<details><summary>Close group billing</summary><p>Stop the subscription and any pending checkout before deleting this group. Remaining monthly credits cannot be transferred.</p>` + strings.Replace(form("end_plan", `<button class="btn-secondary">Stop group billing</button>`), `<form `, `<form data-confirm="Stop group billing now?" `, 1) + `</details>`)
			}
		}
	default:
		cs, _ := group.Conversations(id, acc.ID)
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].Created.Before(cs[j].Created) })
		b.WriteString(`<div class="collection-list">`)
		for _, c := range cs {
			label := "Micro"
			if c.Agent == "none" {
				label = "People only"
			} else if a := agent.For(scope, c.Agent); a != nil {
				label = a.Name
			}
			_, preview, _ := chat.Latest(c.ID)
			b.WriteString(`<a class="collection-item" href="/chat?id=` + c.ID + `"><span class="collection-title">` + html.EscapeString(c.Name) + `</span><span class="text-muted">` + html.EscapeString(label) + `</span><span class="collection-preview">` + html.EscapeString(preview) + `</span></a>`)
		}
		b.WriteString(`</div><details class="record-card"><summary>New chat</summary>` + form("conversation", `<label>Name<input name="name" required maxlength="100"></label>`+groupAgentPicker(scope, g.Encrypted)+`<button>Create chat</button>`) + `</details>`)
	}
	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: g.Name, HTML: b.String()})
}
func groupZoneField() string {
	return `<label>Timezone<input name="zone" value="Europe/London" required placeholder="Europe/London"></label>`
}
func groupAgentPicker(scope string, encrypted bool) string {
	s := `<label>Assistant<select name="agent">`
	if !encrypted {
		s += `<option value="">Micro</option>`
		for _, a := range agent.Agents(scope) {
			s += `<option value="` + html.EscapeString(a.ID) + `">` + html.EscapeString(a.Name) + `</option>`
		}
	}
	return s + `<option value="none">None</option></select></label>`
}
func groupJoinHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id := r.URL.Query().Get("id")
	_, acc := auth.TrySession(r)
	var err error
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if r.ParseForm() != nil {
			http.Error(w, "Invalid form", 400)
			return
		}
		if id == "" {
			id, err = group.InvitationGroup(r.PostFormValue("token"))
			if err != nil {
				app.RespondError(w, 404, "Invitation expired or revoked")
				return
			}
		}
		if r.PostFormValue("action") == "preview" {
			name, inviter, err := group.PreviewLink(id, r.PostFormValue("token"))
			if err != nil {
				app.RespondError(w, 404, "Invitation expired or revoked")
				return
			}
			app.RespondJSON(w, map[string]string{"name": name, "inviter": inviter})
			return
		}
		if !auth.StrictCSRF(r) || acc == nil {
			app.Forbidden(w, r, "Sign in to accept this invitation")
			return
		}
		if err := auth.CheckPostRate(acc.ID); err != nil {
			http.Error(w, err.Error(), 429)
			return
		}
		if err := group.AcceptLink(id, acc.ID, r.PostFormValue("token")); err != nil {
			app.Respond(w, r, app.Response{Title: "Invitation", HTML: app.Problem(err.Error())})
			return
		}
		http.Redirect(w, r, "/groups?id="+id, 303)
		return
	}
	if r.Method != "GET" {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	body := `<div data-group-invitation="` + html.EscapeString(id) + `"><p data-invitation-description>Join a shared group on Micro. Members can read the group's conversation history. Your personal messages stay private.</p>`
	if acc == nil {
		dest := url.QueryEscape(r.URL.RequestURI())
		body += `<div class="form-actions"><a class="btn" href="/login?redirect=` + dest + `">Log in</a><a href="/signup?redirect=` + dest + `">Create account</a></div>`
	} else {
		body += `<form method="POST">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="token" data-invitation-token><button>Accept invitation</button></form>`
	}
	app.Respond(w, r, app.Response{Title: "Group invitation", HTML: body + `</div>`})
}
