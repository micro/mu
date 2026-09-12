package home

import (
	"html"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// appsHTML uses the same pins and service metadata as the sidebar. Defaults
// give a new account useful starting points without displaying the catalogue.
func appsHTML(acc *auth.Account) string {
	apps := app.ServiceShortcuts(acc)
	seen := make(map[string]bool)
	for _, s := range apps {
		seen[s.Name] = true
	}
	for _, s := range service.Pinned([]string{"news", "video", "web", "mail", "notes", "tasks", "events", "maps", "weather", "markets"}) {
		if !seen[s.Name] {
			apps = append(apps, s)
			seen[s.Name] = true
		}
	}
	if len(apps) > 10 {
		apps = apps[:10]
	}
	var b strings.Builder
	b.WriteString(`<div class="home-apps-grid">`)
	for _, s := range apps {
		b.WriteString(`<a href="` + html.EscapeString(s.Page) + `"><span class="home-app-icon"><img src="/` + html.EscapeString(s.NavIcon()) + `" alt=""></span><span>` + html.EscapeString(s.NavLabel()) + `</span></a>`)
	}
	b.WriteString(`<a class="home-apps-all" href="/services" aria-label="All services"><span class="home-app-icon" aria-hidden="true">→</span><span>All services</span></a></div>`)
	return `<nav class="home-apps page-stack" aria-label="Services">` + app.PreviewCard("home-services-card", "Services", "/services", b.String()) + `</nav>`
}
