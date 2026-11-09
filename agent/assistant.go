package agent

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/url"
	"strings"
)

func assistantPath(owner, selected string) string { return "/assistant/" + SlugFor(owner, selected) }

// AssistantHandler is the focused browser client. Execution, history and result
// rendering remain shared with the console and other clients.
func AssistantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	_, acc := auth.TrySession(r)
	if acc == nil {
		app.RedirectToLogin(w, r)
		return
	}
	view := r.URL.Query().Get("view")
	if r.URL.Path != "/assistant" && r.URL.Path != "/assistant/" {
		if view != "" {
			http.NotFound(w, r)
			return
		}
		consoleHandler(w, r, true)
		return
	}
	body, title := "", "Agents"
	switch view {
	case "":
		q := url.Values{}
		for _, key := range []string{"session", "continue", "new"} {
			if value := r.URL.Query().Get(key); value != "" {
				q.Set(key, value)
			}
		}
		selected := ""
		if ref := r.URL.Query().Get("agent"); ref != "" {
			var ok bool
			selected, ok = BySlug(acc.ID, ref)
			if !ok {
				http.NotFound(w, r)
				return
			}
		}
		target := assistantPath(acc.ID, selected)
		if len(q) > 0 {
			target += "?" + q.Encode()
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	case "agents":
		body = `<h1>Choose an agent</h1><div class="collection-list">`
		for _, a := range ClientAgents(acc.ID) {
			target := "/assistant/" + a.Slug
			if r.URL.Query().Get("new") == "1" {
				target += "?new=1"
			}
			body += `<a class="collection-item" href="` + html.EscapeString(target) + `"><div><strong>` + html.EscapeString(a.Name) + `</strong><p class="text-muted">` + html.EscapeString(a.Description) + `</p></div></a>`
		}
		body += `</div>`
	case "chats":
		title = "Chats"
		chats := []thread.Thread{}
		for _, t := range thread.List(acc.ID, 0) {
			if t.Client == thread.WebClient {
				chats = append(chats, t)
			}
		}
		pager := app.Paginate(r, len(chats), app.PerPage)
		body = `<h1>Chats</h1><div class="form-actions">` + app.ActionLink("/assistant?view=agents&new=1", "New chat") + `</div><div class="collection-list">`
		for _, t := range chats[pager.From:pager.To] {
			subject := strings.TrimSpace(t.Subject)
			if subject == "" {
				subject = "Untitled chat"
			}
			body += `<a class="collection-item" href="` + html.EscapeString(assistantPath(acc.ID, t.Agent)+"?session="+url.QueryEscape(t.ID)) + `"><div><strong>` + html.EscapeString(subject) + `</strong><p class="text-muted">` + html.EscapeString(app.TimeAgo(t.Updated)) + `</p></div></a>`
		}
		body += `</div>` + pager.Nav("/assistant")
		if len(chats) == 0 {
			body += `<p>Your conversations will appear here.</p>`
		}
	case "account":
		title = "Account"
		body = `<h1>Account</h1><p>` + html.EscapeString(acc.ID) + `</p><div class="collection-list"><a class="collection-item" href="/account">Account and billing</a><a class="collection-item" href="/home">Open runtime console</a><a class="collection-item" href="/download">Install Mu</a><a class="collection-item" href="/logout">Sign out</a></div><p class="text-muted">Account settings and agent creation are managed in the runtime console.</p>`
	default:
		http.NotFound(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, app.AssistantHTML(title, body, acc, r.URL.RequestURI()))
}
