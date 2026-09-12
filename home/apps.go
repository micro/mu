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
	for _, s := range service.Pinned([]string{"events", "notes", "docs", "files", "contacts", "maps", "apps"}) {
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
	b.WriteString(`<a class="home-apps-all" href="/services" aria-label="All services"><span class="home-app-icon" aria-hidden="true">→</span><span>See all</span></a></div>`)
	return `<nav class="home-apps" aria-label="Services"><div class="section-card-head"><h4><a class="card-head-link" href="/services">Services</a></h4></div>` + b.String() + `</nav>`
}
