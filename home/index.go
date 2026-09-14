package home

import (
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

// Index keeps the conversation at the same address before and after sign-in.
func Index(w http.ResponseWriter, r *http.Request) {
	if _, acc := auth.TrySession(r); acc != nil {
		agent.Handler(w, r)
		return
	}
	page := app.RenderIndex(app.Index{
		Title: "Home | Micro", Description: "A personal AI agent",
		TopRight: topRight(), Body: indexBody(), Footer: app.FooterLinks(), Tail: workerScript(),
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Write([]byte(page))
}

func indexBody() string {
	return `<div class="conversation-entry"><div class="brand">Micro</div><p class="text-muted">A personal AI agent</p>` + app.ChatComponent(app.ChatConfig{
		Ask: true, Placeholder: "What do you need?", StorageNS: "landing", Location: true,
	}) + `</div>`
}
func topRight() string { return `<a href="/login?redirect=%2F">Log in</a>` }

func workerScript() string {
	return `<script>
(function () {
  if (!navigator.serviceWorker) return;
  // updateViaCache:'none', the same as the app shell — see internal/app. The
  // default consults the HTTP cache for the worker script, which is how a
  // device ends up running a months-old copy.
  navigator.serviceWorker.register('/mu.js', {scope: '/', updateViaCache: 'none'})
    .then(function (reg) { if (reg && reg.update) reg.update(); })
    .catch(function () {});
})();
</script>`
}
