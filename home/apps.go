package home

import (
	"html"
	"strings"

	"mu/internal/auth"
	"mu/internal/service"
)

// appsHTML uses the same pins and service metadata as the sidebar. Defaults
// give a new account useful starting points without displaying the catalogue.
func appsHTML(acc *auth.Account) string {
	names := []string{"notes", "tasks", "events", "mail", "chat", "web", "video"}
	if acc != nil {
		names = acc.PinnedServices()
	}
	apps := service.Pinned(names)
	if len(apps) > 7 {
		apps = apps[:7]
	}
	var b strings.Builder
	b.WriteString(`<nav class="home-apps" aria-label="Apps"><div class="home-apps-heading">Apps</div><div class="home-apps-grid">`)
	for _, s := range apps {
		b.WriteString(`<a href="` + html.EscapeString(s.Page) + `"><img src="/` + html.EscapeString(s.NavIcon()) + `" alt=""><span>` + html.EscapeString(s.NavLabel()) + `</span></a>`)
	}
	b.WriteString(`<a href="/services"><span class="home-apps-all" aria-hidden="true">⋯</span><span>All services</span></a></div></nav>`)
	return b.String()
}
