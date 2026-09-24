package agent

import (
	"encoding/json"
	"fmt"
	"html"
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
		TitleHandler(w, r)
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
		selected, ok = BySlug(acc.ID, ref)
		if !ok {
			app.NotFound(w, r, "Agent not found")
			return
		}
	}
	if r.URL.Path == "/" && ref != "" && r.URL.Query().Get("session") == "" && r.URL.Query().Get("continue") == "" {
		http.Redirect(w, r, Path(acc.ID, selected), http.StatusSeeOther)
		return
	}
	session := r.URL.Query().Get("session")
	if session == "" {
		session = r.URL.Query().Get("continue")
	}
	if session == "" && acc != nil && r.URL.Query().Get("new") != "1" {
		if recent := RecentConversation(acc.ID, selected); recent != nil {
			http.Redirect(w, r, Path(acc.ID, selected)+"?session="+url.QueryEscape(recent.ID), http.StatusSeeOther)
			return
		}
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
		if selected == DefaultPlatformAgent {
			selected = ""
		}
		target := Path(acc.ID, selected)
		if r.URL.Path != target {
			http.Redirect(w, r, target+"?session="+url.QueryEscape(session), http.StatusSeeOther)
			return
		}
		thread.MarkSeen(acc.ID, session)
		messages := thread.Messages(acc.ID, session, 100)
		delivered := map[string]bool{}
		for _, message := range messages {
			if strings.HasPrefix(message.Ref, "app-build:") {
				delivered[strings.TrimPrefix(message.Ref, "app-build:")] = true
			}
		}
		for _, message := range messages {
			results := message.Results[:0:0]
			for _, item := range message.Results {
				if item.Kind != "app-build" || !delivered[item.ID] {
					results = append(results, item)
				}
			}

			class := "answer"
			content := app.RenderString(message.Text) + app.Results(results)
			if message.Role == thread.RolePerson {
				class = "request"
				content = html.EscapeString(message.Text)
			}
			who := "You"
			if message.Role == thread.RoleAgent {
				who = "Micro"
				if a := For(acc.ID, selected); a != nil {
					who = a.Name
				} else if a := Platform(selected); a != nil {
					who = a.Name
				}
			}
			content = `<div class="ib-from metadata-row"><span class="ib-who-l">` + html.EscapeString(who) + `</span><time class="ib-at" datetime="` + message.At.Format(time.RFC3339) + `" title="` + message.At.Format("2 Jan 2006, 15:04 MST") + `">` + html.EscapeString(app.TimeAgo(message.At)) + `</time></div><div class="message-body">` + content + `</div>`
			delivery := ""
			if strings.HasPrefix(message.Ref, "app-build:") {
				delivery = ` data-delivery-id="` + html.EscapeString(message.Ref) + `"`
			}
			initial += `<section class="turn"` + delivery + `><div class="` + class + `">` + content + `</div></section>`
		}
	}
	if acc != nil {
		if a := For(acc.ID, selected); a != nil {
			agentName = a.Name
			agentDescription = strings.TrimSpace(a.Description)
		} else if a := Platform(selected); a != nil {
			agentName = a.Name
			agentDescription = strings.TrimSpace(a.Description)
		}
	}
	if acc == nil {
		app.RedirectToLogin(w, r)
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
	basePath := Path(acc.ID, selected)
	description := ""
	if agentDescription != "" {
		description = `<p>` + html.EscapeString(agentDescription) + `</p>`
	}
	body := consoleBody(acc.ID, selected, session, agentName, description, initial, heading, state, basePath)
	fmt.Fprint(w, app.ConsoleHTML(agentName, body, acc, r.URL.RequestURI()))
}

// TitleHandler changes only the signed-in owner's web thread.
func TitleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
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

// Prompt embeds the same request-driven composer on Home. Execution remains in Agent.
func Prompt(owner string) string {
	return consoleBody(owner, "", "", "Micro", "", "", "", "conversation", Path(owner, ""))
}

func consoleBody(owner, selected, session, agentName, description, initial, heading, state, basePath string) string {
	toolbar := `<div class="assistant-toolbar"><a href="/agents">Conversations</a><a href="` + html.EscapeString(basePath) + `?new=1">New conversation</a></div>`
	return `<div class="assistant-workspace"><div class="` + state + `">` + toolbar + heading + `<div id="responses" role="log" aria-label="Conversation">` + initial + `</div><div class="prompt-panel"><div class="prompt-welcome"><h1>` + html.EscapeString(agentName) + `</h1>` + description + `</div><form id="command-form" data-path="` + html.EscapeString(basePath) + `" data-account="` + html.EscapeString(owner) + `" data-pending="` + fmt.Sprint(session != "" && Pending(owner, session)) + `" data-agent="` + html.EscapeString(selected) + `" data-agent-name="` + html.EscapeString(agentName) + `"><label class="sr-only" for="command-input">Message</label><div class="composer"><textarea id="command-input" rows="1" maxlength="8000" placeholder="What do you need?" required></textarea><button id="send" type="submit" aria-label="Send message">Send</button></div><p id="status" role="status"></p></form></div></div></div>`
}

// RecentConversation resumes the owner's most recently visited web conversation
// for this agent. Replies in another thread do not displace a visited session.
func RecentConversation(owner, selected string) *thread.Thread {
	if selected == DefaultPlatformAgent {
		selected = ""
	}
	var recent *thread.Thread
	for _, th := range thread.List(owner, 0) {
		id := th.Agent
		if id == DefaultPlatformAgent {
			id = ""
		}
		if th.Client != thread.WebClient || id != selected || th.Seen.IsZero() {
			continue
		}
		if recent == nil || th.Seen.After(recent.Seen) {
			copy := th
			recent = &copy
		}
	}
	return recent
}
