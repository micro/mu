package home

import (
	"fmt"
	"html"
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/url"
)

// ConsoleHandler is the web front door. Nothing runs until a request is sent.
func ConsoleHandler(w http.ResponseWriter, r *http.Request) {
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
	agentDescription := ""
	if a := agent.Platform(""); a != nil {
		agentDescription = a.Description
	}
	if ref := r.URL.Query().Get("agent"); ref != "" {
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
	session := r.URL.Query().Get("session")
	if session == "" {
		session = r.URL.Query().Get("continue")
	}
	if session != "" {
		if acc == nil || thread.Get(acc.ID, session) == nil {
			http.Error(w, "Conversation not found", 404)
			return
		}
		selected = thread.Get(acc.ID, session).Agent
		thread.MarkSeen(acc.ID, session)
		for _, message := range thread.Messages(acc.ID, session, 100) {
			class := "answer"
			content := app.RenderString(message.Text) + app.Results(message.Results)
			if message.Role == thread.RolePerson {
				class = "request"
				content = html.EscapeString(message.Text)
			}
			initial += `<section class="turn"><div class="` + class + `">` + content + `</div></section>`
		}
	}
	if acc != nil {
		if a := agent.For(acc.ID, selected); a != nil {
			agentName, agentDescription = a.Name, a.Description
		} else if a := agent.Platform(selected); a != nil {
			agentName, agentDescription = a.Name, a.Description
		}
	}
	description := ""
	if agentDescription != "" {
		description = `<p>` + html.EscapeString(agentDescription) + `</p>`
	}
	if acc == nil {
		fmt.Fprint(w, app.ConsoleHTML("Micro", `<div class="conversation"><div class="prompt-panel"><div class="prompt-welcome"><h1>Micro</h1>`+description+`<p><a class="btn" href="/login">Sign in to talk to Micro</a></p></div></div></div>`, acc))
		return
	}
	newConversation := "/"
	if selected != "" && acc != nil {
		newConversation = "/?agent=" + url.QueryEscape(agent.SlugFor(acc.ID, selected))
	}
	state := "conversation"
	if session != "" {
		state += " is-active"
	}
	fmt.Fprint(w, app.ConsoleHTML("Micro", `<div class="`+state+`"><div class="prompt-panel"><div class="conversation-actions"><span>`+html.EscapeString(agentName)+`</span><a href="`+html.EscapeString(newConversation)+`">New conversation</a></div><div class="prompt-welcome"><h1>`+html.EscapeString(agentName)+`</h1>`+description+`</div><form id="command-form" data-agent="`+html.EscapeString(selected)+`"><label class="sr-only" for="command-input">Command or question</label><div class="composer"><input type="text" id="command-input" maxlength="8000" placeholder="What do you need?" autocomplete="off" required><button id="send" type="submit" aria-label="Send command">Send</button></div><p id="status" role="status"></p></form></div><div id="responses" role="log" aria-label="Requests and responses">`+initial+`</div></div>`, acc))
}
