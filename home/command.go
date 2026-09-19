package home

import (
	"encoding/json"
	"fmt"
	"html"
	"mu/agent"
	"mu/agent/hello"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// ConsoleHandler is the web front door. Nothing runs until a request is sent.
func ConsoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		renameThread(w, r)
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, acc := auth.TrySession(r)
	initial := ""
	selected, agentName := "", "Micro"
	agentDescription := "A personal assistant"

	ref := r.URL.Query().Get("agent")
	if strings.HasPrefix(r.URL.Path, "/agent/") {
		ref = strings.TrimPrefix(r.URL.Path, "/agent/")
	}
	if ref != "" {
		if acc == nil {
			app.RedirectToLogin(w, r)
			return
		}
		var ok bool
		selected, ok = agent.BySlug(acc.ID, ref)
		if !ok {
			app.NotFound(w, r, "Agent not found")
			return
		}
	}
	if r.URL.Path == "/" && ref != "" && r.URL.Query().Get("session") == "" && r.URL.Query().Get("continue") == "" {
		http.Redirect(w, r, agent.Path(acc.ID, selected), http.StatusSeeOther)
		return
	}
	session := r.URL.Query().Get("session")
	if session == "" {
		session = r.URL.Query().Get("continue")
	}
	if session != "" {
		if acc == nil || thread.Get(acc.ID, session) == nil {
			http.Error(w, "Conversation not found", 404)
			return
		}
		if thread.Get(acc.ID, session).Client != thread.WebClient {
			http.Redirect(w, r, "/inbox?id="+url.QueryEscape(session), http.StatusSeeOther)
			return
		}
		selected = thread.Get(acc.ID, session).Agent
		if selected == agent.DefaultPlatformAgent {
			selected = ""
		}
		target := agent.Path(acc.ID, selected)
		if (r.URL.Path != "/" && r.URL.Path != target) || (r.URL.Path == "/" && selected != "") {
			http.Redirect(w, r, target+"?session="+url.QueryEscape(session), http.StatusSeeOther)
			return
		}
		thread.MarkSeen(acc.ID, session)
		for _, message := range thread.Messages(acc.ID, session, 100) {
			class := "answer"
			content := app.RenderString(message.Text) + app.Results(message.Results)
			if message.Role == thread.RolePerson {
				class = "request"
				content = html.EscapeString(message.Text)
			}
			who := "You"
			if message.Role == thread.RoleAgent {
				who = "Micro"
				if a := agent.For(acc.ID, selected); a != nil {
					who = a.Name
				} else if a := agent.Platform(selected); a != nil {
					who = a.Name
				}
			}
			content = `<div class="ib-from metadata-row"><span class="ib-who-l">` + html.EscapeString(who) + `</span><time class="ib-at" datetime="` + message.At.Format(time.RFC3339) + `" title="` + message.At.Format("2 Jan 2006, 15:04 MST") + `">` + html.EscapeString(app.TimeAgo(message.At)) + `</time></div><div class="message-body">` + content + `</div>`
			initial += `<section class="turn"><div class="` + class + `">` + content + `</div></section>`
		}
	}
	if acc != nil {
		if a := agent.For(acc.ID, selected); a != nil {
			agentName = a.Name
			agentDescription = strings.TrimSpace(a.Description)
		} else if a := agent.Platform(selected); a != nil {
			agentName = a.Name
			agentDescription = strings.TrimSpace(a.Description)
		}
	}
	if acc == nil {
		fmt.Fprint(w, app.ConsoleHTML("Micro", `<div class="conversation"><div class="prompt-panel"><div class="prompt-welcome"><h1>Micro</h1><p>A personal assistant</p></div></div></div>`, acc))
		return
	}

	state := "conversation"
	if session != "" {
		state += " is-active"
	}
	heading := ""
	if session != "" {
		heading = `<h1 class="assistant-thread-title"><button type="button" data-edit-thread title="Rename thread">` + html.EscapeString(thread.Get(acc.ID, session).Subject) + `</button></h1>`
	}
	hello.ClaimAccount(acc.ID)
	basePath := "/"
	if strings.HasPrefix(r.URL.Path, "/agent/") || selected != "" {
		basePath = agent.Path(acc.ID, selected)
	}
	description := ""
	if agentDescription != "" {
		description = `<p>` + html.EscapeString(agentDescription) + `</p>`
	}
	history := assistantHistory(acc.ID, session, selected, basePath)
	body := `<div class="assistant-workspace">` + history + `<div class="` + state + `">` + heading + `<div id="responses" role="log" aria-label="Conversation">` + initial + `</div><div class="prompt-panel"><div class="prompt-welcome"><h1>` + html.EscapeString(agentName) + `</h1>` + description + `</div><form id="command-form" data-path="` + html.EscapeString(basePath) + `" data-account="` + html.EscapeString(acc.ID) + `" data-pending="` + fmt.Sprint(session != "" && agent.Pending(acc.ID, session)) + `" data-agent="` + html.EscapeString(selected) + `" data-agent-name="` + html.EscapeString(agentName) + `"><label class="sr-only" for="command-input">Message</label><div class="composer"><textarea id="command-input" rows="1" maxlength="8000" placeholder="What do you need?" required></textarea><button id="send" type="submit" aria-label="Send message">Send</button></div><p id="status" role="status"></p></form></div></div></div>`
	fmt.Fprint(w, app.ConsoleHTML(agentName, body, acc))
}

func assistantHistory(owner, selected, agentID, basePath string) string {
	var b strings.Builder
	b.WriteString(`<aside id="assistant-history" class="assistant-history" hidden><div class="assistant-toolbar"><strong>Recent</strong><a href="` + html.EscapeString(basePath) + `?new=1">New</a></div><nav aria-label="Recent">`)
	count := 0
	for _, t := range thread.List(owner, 0) {
		target := t.Agent
		if target == agent.DefaultPlatformAgent {
			target = ""
		}
		if t.Client != thread.WebClient || target != agentID {
			continue
		}
		if count == 10 {
			break
		}
		count++
		current := ""
		if t.ID == selected {
			current = ` aria-current="page"`
		}
		title := strings.TrimSpace(t.Subject)
		if title == "" {
			title = "Untitled"
		}
		b.WriteString(`<a class="conversation-row" href="` + html.EscapeString(basePath) + `?session=` + url.QueryEscape(t.ID) + `"` + current + `><span class="conversation-title">` + html.EscapeString(title) + `</span><time>` + html.EscapeString(app.TimeAgo(t.Updated)) + `</time></a>`)
	}
	if count == 0 {
		b.WriteString(`<p class="text-muted">No threads yet.</p>`)
	}
	b.WriteString(`</nav></aside>`)
	return b.String()
}

// renameThread changes only the signed-in owner's web thread.
func renameThread(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	if acc == nil {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil || !auth.StrictCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}
	title := strings.TrimSpace(r.PostForm.Get("title"))
	if r.PostForm.Get("action") != "rename-thread" || title == "" || utf8.RuneCountInString(title) > 160 {
		http.Error(w, "Enter a title of 1–160 characters", http.StatusBadRequest)
		return
	}
	if !thread.RetitleWeb(acc.ID, r.PostForm.Get("thread"), title) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]string{"title": title})
}
