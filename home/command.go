package home

import (
	"fmt"
	"html"
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"time"
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
		fmt.Fprint(w, app.ConsoleHTML("Micro", `<div class="conversation"><div class="prompt-panel"><div class="prompt-welcome"><h1>Micro</h1><p>A personal assistant for everyone.</p><p class="landing-example">Research a trip, remember something, or set a reminder.</p></div><form id="guest-command-form" action="/signup" method="get"><label class="sr-only" for="guest-command-input">Message</label><div class="composer"><input type="text" id="guest-command-input" maxlength="8000" placeholder="Write a message…" autocomplete="off" aria-describedby="guest-status" required><button type="submit">Continue</button></div><p id="guest-status" class="composer-note" role="status">Create an account to send your message.</p></form></div></div>`, acc))
		return
	}
	state := "conversation"
	if session != "" || r.URL.Query().Get("new") == "1" {
		state += " is-active"
	}
	fmt.Fprint(w, app.ConsoleHTML("Micro", `<div class="`+state+`"><div class="prompt-panel"><div class="prompt-welcome"><h1>`+html.EscapeString(agentName)+`</h1>`+description+`</div><form id="command-form" data-pending="`+fmt.Sprint(session != "" && agent.Pending(acc.ID, session))+`" data-agent="`+html.EscapeString(selected)+`" data-agent-name="`+html.EscapeString(agentName)+`"><label class="sr-only" for="command-input">Message</label><div class="composer"><input type="text" id="command-input" maxlength="8000" placeholder="Write a message…" autocomplete="off" required><button id="send" type="submit" aria-label="Send message">Send</button></div><p id="status" role="status"></p></form></div><div id="responses" role="log" aria-label="Requests and responses">`+initial+`</div></div>`, acc))
}
