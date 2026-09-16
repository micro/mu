package home

import (
	"fmt"
	"html"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
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
	session := r.URL.Query().Get("session")
	if session == "" {
		session = r.URL.Query().Get("continue")
	}
	if session != "" {
		if acc == nil || thread.Get(acc.ID, session) == nil {
			http.Error(w, "Conversation not found", 404)
			return
		}
		thread.MarkSeen(acc.ID, session)
		for _, message := range thread.Messages(acc.ID, session, 100) {
			class := "answer"
			content := app.RenderString(message.Text)
			if message.Role == thread.RolePerson {
				class = "request"
				content = html.EscapeString(message.Text)
			}
			initial += `<section class="turn"><div class="` + class + `">` + content + `</div></section>`
		}
	}
	state := "conversation"
	if session != "" {
		state += " is-active"
	}
	fmt.Fprint(w, app.ConsoleHTML("Micro", `<div class="`+state+`"><div class="prompt-panel"><div class="conversation-actions"><a href="/">New conversation</a></div><div class="prompt-welcome"><h1>Micro</h1><p>A personal assistant</p></div><form id="command-form"><label class="sr-only" for="command-input">Command or question</label><div class="composer"><input type="text" id="command-input" maxlength="8000" placeholder="What do you need?" autocomplete="off" required><button id="send" type="submit" aria-label="Send command">Send</button></div><p id="status" role="status"></p></form></div><div id="responses" role="log" aria-label="Requests and responses">`+initial+`</div></div>`, acc))
}
