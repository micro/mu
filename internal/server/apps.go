package server

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/service"
)

// appIntroduction only handles the front door of an app. Resource URLs,
// forms, JSON requests and protocols continue through their normal handlers.
func appIntroduction(w http.ResponseWriter, r *http.Request, protected bool) bool {
	if r.Method != http.MethodGet || r.URL.RawQuery != "" || app.WantsJSON(r) || app.SendsJSON(r) {
		return false
	}
	descriptions := map[string]string{
		"mail":    "Read and send email, keep conversations together, and ask Micro to help you understand or draft a reply.",
		"events":  "Keep track of your schedule and set reminders. Ask Micro what is coming up or remind you about something that matters.",
		"notes":   "Capture a thought, keep a list or save something you want to remember. Your notes are there when you or Micro need them.",
		"files":   "Upload and keep files in one place, then find and use them again in your conversations with Micro.",
		"docs":    "Create, import and edit documents. Work on them directly or ask Micro to help with the writing.",
		"news":    "Read the latest headlines and explore the subjects you follow. Ask Micro to help you understand what is happening.",
		"markets": "Follow market prices and the things you are watching. Ask Micro to look up information and explain it.",
		"video":   "Find and watch videos, and save useful things to return to later.",
		"weather": "Check the weather and forecast for a place. Ask Micro for the conditions that matter to your plans.",
		"maps":    "Explore a place on the map and get directions. Use it directly or ask Micro to help you plan a journey.",
	}
	for _, s := range service.Nav() {
		if s.Page != r.URL.Path || s.Page == "" {
			continue
		}
		description, featured := descriptions[s.Name]
		if !featured && !protected {
			return false
		}
		if description == "" {
			description = s.Description
		}
		var b strings.Builder
		b.WriteString(`<section class="app-introduction"><span class="app-launcher-icon"><img src="/` + html.EscapeString(s.NavIcon()) + `" width="32" height="32" alt=""></span><div class="prompt-welcome"><h1>` + html.EscapeString(s.NavLabel()) + `</h1><p>A built-in app for your everyday life.</p></div><p>` + html.EscapeString(description) + `</p><p>Log in or create an account to use it with Micro.</p><div class="form-actions"><a class="btn" href="/login?redirect=` + url.QueryEscape(s.Page) + `">Log in</a><a class="btn" href="/signup?redirect=` + url.QueryEscape(s.Page) + `">Create an account</a></div>`)
		browse := s.Name == "news" || s.Name == "markets" || s.Name == "video" || s.Name == "weather" || s.Name == "maps"
		if !protected && browse {
			b.WriteString(`<a data-app-browse href="` + html.EscapeString(s.Page) + `?view=public">Browse ` + html.EscapeString(s.NavLabel()) + `</a>`)
		}
		b.WriteString(`<a href="/services">Explore all apps</a></section>`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Add("Vary", "Accept")
		w.Write([]byte(app.ConsoleHTML(s.NavLabel(), b.String(), nil, r.URL.Path)))
		return true
	}
	return false
}
