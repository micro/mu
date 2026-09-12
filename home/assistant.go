package home

import (
	"fmt"
	"net/http"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
)

// AssistantHandler offers the full conversation independently of Home. The
// embedded view is the same trusted page, retained while its panel is closed.
func AssistantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	_, acc := auth.TrySession(r)
	ns := "assistant:guest"
	if acc != nil {
		ns = "assistant:account:" + acc.ID
	}
	w.Header().Set("Cache-Control", "private, no-store")
	body := `<main class="assistant-page"><script>window.muActiveAgent="";</script>` + app.ChatComponent(app.ChatConfig{
		Ask: true, HideSuggestions: true, AgentName: agent.DefaultName(),
		Placeholder: "What do you need?", Location: acc != nil, StorageNS: ns,
	}) + `</main>`
	if r.URL.Query().Get("panel") == "1" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html lang="en"><head><title>Assistant</title><meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content"><meta name="referrer" content="no-referrer"><base target="_top"><link rel="stylesheet" href="/mu.css?`+app.Version+`"><link rel="stylesheet" href="/composition.css?`+app.Version+`"><link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:wght@400;600;700&display=swap" rel="stylesheet"><script src="/mu.js?`+app.Version+`"></script></head><body class="assistant-embedded">`+body+`<script>document.addEventListener('keydown',function(e){if(e.key==='Escape' && parent!==window){var panel=parent.document.getElementById('assistant-panel');if(panel){e.preventDefault();panel.close();}}});</script></body></html>`)
		return
	}
	body = `<div class="section-actions"><a href="/home">Home</a></div>` + body
	app.Respond(w, r, app.Response{Title: "Assistant", Description: "Talk to Micro", HTML: body})
}
