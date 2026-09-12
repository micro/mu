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
	ns := assistantNamespace(viewerID)
	fromHome := r.URL.Query().Get("view") == "home"
	if fromHome {
		ns += ":home"
	}
	w.Header().Set("Cache-Control", "private, no-store")
	body := `<main class="assistant-page"><script>window.muActiveAgent="";</script>` + app.ChatComponent(app.ChatConfig{
		Ask: true, HideSuggestions: true, AgentName: agent.DefaultName(),
		Placeholder: "What do you need?", Location: acc != nil, StorageNS: ns, AcceptHandoff: fromHome,
	}) + `</main>`
	if fromHome {
		body = `<div class="page-stack"><div class="section-actions"><a href="/assistant">Back to main conversation</a></div>` + body + `</div>`
	}
	app.Respond(w, r, app.Response{Title: "Assistant", Description: "Talk to Micro", HTML: body})
}

func assistantNamespace(account string) string {
	if account == "" {
		return "assistant:guest"
	}
	return "assistant:account:" + account
}
