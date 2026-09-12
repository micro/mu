package home

import (
	"net/http"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
)

// AssistantHandler offers the ongoing conversation independently of Home.
func AssistantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	_, acc := auth.TrySession(r)
	viewerID := ""
	if acc != nil {
		viewerID = acc.ID
	}
	if acc != nil {
		auth.SetCSRFCookie(w, r)
		agent.Handler(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	body := `<main class="assistant-page">` + app.ChatComponent(app.ChatConfig{
		Ask: true, HideSuggestions: true, AgentName: agent.DefaultName(), Transcript: true,
		Placeholder: "What do you need?", StorageNS: assistantNamespace(viewerID),
	}) + `</main>`
	app.Respond(w, r, app.Response{Title: "Assistant", HTML: body})
}

func assistantNamespace(account string) string {
	if account == "" {
		return "assistant:guest"
	}
	return "assistant:account:" + account
}
