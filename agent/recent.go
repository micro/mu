package agent

import (
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/group"
	"mu/internal/thread"
	"mu/service/tasks"
)

func activityAgentName(owner, id string) string {
	if id == "" || id == DefaultPlatformAgent {
		return DefaultName()
	}
	return NameOf(owner, id)
}

// RecentActivity composes existing conversations and work without storing a
// second feed. Group membership is checked on every read.
func RecentActivity(owner string, arrivals bool) []inbox.Chat {
	chats := inbox.Chats(owner)
	rooms := map[string]bool{}
	for _, c := range chats {
		if c.Shared {
			rooms[c.ID] = true
		}
	}
	if arrivals {
		for _, t := range thread.List(owner, 0) {
			if !thread.Arrived(t) || (t.Client == thread.ChatClient && rooms[t.Key]) {
				continue
			}
			preview := ""
			if messages := thread.Messages(owner, t.ID, 1); len(messages) > 0 {
				preview = inbox.PreviewText(messages[0].Text)
			}
			title := t.Subject
			if title == "" {
				title = "Untitled"
			}
			chats = append(chats, inbox.Chat{ID: t.ID, Title: title, Agent: t.Agent, Context: app.ClientName(t.Client), URL: "/inbox?id=" + url.QueryEscape(t.ID), Updated: t.Updated, Preview: preview, Unread: thread.Unread(t)})
		}
	}
	index := map[string]int{}
	for i := range chats {
		c := &chats[i]
		scope := owner
		key := c.ID
		if c.Shared {
			scope = group.Scope(c.Group)
			if t := thread.Find(scope, thread.ChatClient, c.ID); t != nil {
				key = t.ID
			}
		}
		index[scope+"/"+key] = i
		name := activityAgentName(scope, c.Agent)
		if name != "" && c.Agent != "none" {
			c.Context = name + " · " + c.Context
		}
	}
	scopes := []string{owner}
	if gs, _, err := group.List(owner); err == nil {
		for _, g := range gs {
			scopes = append(scopes, group.Scope(g.ID))
		}
	}
	for _, scope := range scopes {
		work := tasks.List(scope, "")
		sort.SliceStable(work, func(i, j int) bool {
			if work[i].Open() != work[j].Open() {
				return work[i].Open()
			}
			return work[i].Updated.After(work[j].Updated)
		})
		seen := map[string]bool{}
		for _, t := range work {
			if t.Archived || t.Assignee != tasks.Agent || t.Occurrence != nil {
				continue
			}
			key := scope + "/" + t.Thread
			if i, ok := index[key]; ok {
				if seen[key] {
					continue
				}
				seen[key] = true
				c := &chats[i]
				c.Status = t.Status
				newer := !t.Updated.Before(c.Updated)
				if newer {
					c.Updated = t.Updated
				}
				if t.Result != "" && (newer || t.Open()) {
					c.Preview = inbox.PreviewText(t.Result)
				} else if newer || t.Open() {
					c.Preview = inbox.PreviewText(t.Title)
				}
			} else if arrivals && scope == owner {
				chats = append(chats, inbox.Chat{ID: "work:" + t.ID, Title: t.Title, Context: activityAgentName(owner, t.Agent), Agent: t.Agent, URL: "/work?id=" + url.QueryEscape(t.ID), Updated: t.Updated, Status: t.Status, Preview: inbox.PreviewText(t.Result)})
			}
		}
	}
	sort.SliceStable(chats, func(i, j int) bool {
		if chats[i].Updated.Equal(chats[j].Updated) {
			return chats[i].ID < chats[j].ID
		}
		return chats[i].Updated.After(chats[j].Updated)
	})
	return chats
}

func chatsPage(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	title, description := "Chats", ""
	selected := r.URL.Query().Get("agent")
	groupID := r.URL.Query().Get("group")
	groupName := ""
	scope := acc.ID
	if groupID != "" {
		g, err := group.Read(groupID, acc.ID)
		if err != nil {
			app.NotFound(w, r, "Group not found")
			return
		}
		scope = group.Scope(g.ID)
		groupName = g.Name
		title = g.Name + " chats"
	}
	if selected != "" {
		id, ok := BySlug(scope, selected)
		if !ok {
			app.NotFound(w, r, "Agent not found")
			return
		}
		selected = id
		title = activityAgentName(scope, id) + " chats"
		if a := For(scope, id); a != nil {
			description = a.Description
		} else if a := Platform(id); a != nil {
			description = a.Description
		}
	}
	if r.URL.Query().Has("agent") && groupName != "" {
		title += " · " + groupName
	}
	chats := RecentActivity(acc.ID, false)
	filtered := chats[:0]
	for _, c := range chats {
		if groupID != "" && c.Group != groupID {
			continue
		}
		if r.URL.Query().Has("agent") && (SlugFor(scope, c.Agent) != SlugFor(scope, selected) || (groupID == "" && c.Shared)) {
			continue
		}
		filtered = append(filtered, c)
	}
	chats = filtered
	pager := app.Paginate(r, len(chats), 25)
	newURL := "/agent?view=new-chat"
	if groupID != "" {
		newURL += "&group=" + url.QueryEscape(groupID)
	}
	if selected != "" {
		newURL += "&agent=" + url.QueryEscape(selected)
	}
	actions := `<div class="form-actions"><a class="btn" href="` + html.EscapeString(newURL) + `">New chat</a>`
	if r.URL.Query().Has("agent") || groupID != "" {
		actions += `<a href="/agent?view=chats">All chats</a>`
	}
	if groupID != "" {
		actions += `<a href="/groups?id=` + url.QueryEscape(groupID) + `&amp;tab=details">Group details</a>`
	}
	actions += `</div>`
	var b strings.Builder
	b.WriteString(app.PageControls(description, app.AgentViews("agents"), actions))
	if len(chats) == 0 {
		b.WriteString(`<p class="text-muted">Start a conversation with an assistant or your group.</p>`)
	}
	for _, c := range chats[pager.From:pager.To] {
		b.WriteString(inbox.ChatRow(c))
	}
	b.WriteString(pager.Nav(r.URL.Path + "?" + r.URL.Query().Encode()))
	app.Respond(w, r, app.Response{Title: title, HTML: `<div class="page-stack">` + b.String() + `</div>`})
}

func conversationActivityHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		http.Error(w, "Sign in to continue", 401)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	owner, id := acc.ID, r.URL.Query().Get("thread")
	shared := false
	if room := r.URL.Query().Get("room"); room != "" {
		g, _, ok := group.ConversationFor(room)
		if !ok || !group.Member(g.ID, acc.ID) || g.Encrypted {
			app.NotFound(w, r, "Chat not found")
			return
		}
		owner = group.Scope(g.ID)
		shared = true
		if t := thread.Find(owner, thread.ChatClient, room); t != nil {
			id = t.ID
		} else {
			app.RespondJSON(w, map[string]any{"html": ""})
			return
		}
	}
	if thread.Get(owner, id) == nil {
		app.NotFound(w, r, "Chat not found")
		return
	}
	var b strings.Builder
	pending := false
	count := 0
	label := "Done"
	for _, t := range tasks.List(owner, "") {
		if t.Thread != id || t.Archived || t.Assignee != tasks.Agent || t.Occurrence != nil {
			continue
		}
		count++
		if t.Open() || count == 1 {
			label = app.WorkState(t.Status)
		}
		pending = pending || t.Status == tasks.StatusDoing || t.Status == tasks.StatusTodo
		b.WriteString(`<div class="collection-item"><div class="section-stack"><div class="metadata-row">` + app.WorkBadge(t.Status) + `<span>` + html.EscapeString(t.Title) + `</span></div>`)
		if t.Result != "" {
			b.WriteString(`<p class="text-muted">` + html.EscapeString(inbox.PreviewText(t.Result)) + `</p>`)
		}
		if !shared {
			b.WriteString(`<a href="/work?id=` + url.QueryEscape(t.ID) + `">Work details</a>`)
		}
		b.WriteString(`</div></div>`)
	}
	body := b.String()
	if count > 0 {
		body = `<details><summary>Work · ` + html.EscapeString(label) + `</summary><div class="collection-list">` + body + `</div></details>`
	}
	app.RespondJSON(w, map[string]any{"html": body, "pending": pending})
}
