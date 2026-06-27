package home

import (
	"net/http"
	"time"

	"mu/internal/app"
)

func AssistantHandler(w http.ResponseWriter, r *http.Request) {
	prefill := r.URL.Query().Get("q")
	if prefill == "" {
		prefill = r.URL.Query().Get("prompt")
	}

	content := ""

	// Once a day, lead with a card-rich morning brief — the same dashboard
	// cards, surfaced as a daily digest on first visit. Skipped when the user
	// arrived via a deep-link query so we don't bury their answer.
	if prefill == "" && shouldShowBrief(r) {
		if brief := MorningBriefHTML(); brief != "" {
			content += `<div id="mu-brief" style="margin-bottom:24px">` + brief + `</div>`
			setBriefSeen(w)
		}
	}

	content += `<div style="margin-bottom:24px">` + chatComponent(false) + `</div>`

	// Auto-submit if a query param was provided (e.g. from a deep link).
	if prefill != "" {
		content += `<script>(function(){
var input=document.getElementById('mu-chat-input');
if(input&&window.muChatAsk){input.value=` + jsString(prefill) + `;window.muChatAsk(input.value);}
history.replaceState(null,'','/');
})()</script>`
	}

	html := app.RenderHTMLForRequest("Micro", "Your personal AI agent", content, r)
	w.Write([]byte(html))
}

// briefDay returns today's date (UTC) as a stable per-day key.
func briefDay() string {
	return time.Now().UTC().Format("2006-01-02")
}

// shouldShowBrief reports whether the daily brief hasn't been shown yet today
// (tracked per-browser via the mu_brief cookie).
func shouldShowBrief(r *http.Request) bool {
	c, err := r.Cookie("mu_brief")
	return err != nil || c.Value != briefDay()
}

// setBriefSeen records that today's brief has been shown.
func setBriefSeen(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "mu_brief",
		Value:    briefDay(),
		Path:     "/",
		Expires:  time.Now().Add(48 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
