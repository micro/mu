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
	names := []string{"news", "video", "web", "mail"}
	if acc != nil && len(acc.Pinned) > 0 {
		names = acc.PinnedServices()
	}
	apps := service.Pinned(names)
	if len(apps) == 0 {
		apps = service.Pinned([]string{"news", "video", "web", "mail"})
	}
	if len(apps) > 7 {
		apps = apps[:7]
	}
	var b strings.Builder
	b.WriteString(`<nav class="home-apps page-stack" aria-label="Services">` + sectionRule("Services") + `<div class="home-apps-grid">`)
	for _, s := range apps {
		b.WriteString(`<a href="` + html.EscapeString(s.Page) + `"><span class="home-app-icon"><img src="/` + html.EscapeString(s.NavIcon()) + `" alt=""></span><span>` + html.EscapeString(s.NavLabel()) + `</span></a>`)
	}
	b.WriteString(`</div><a href="/services" class="link">Go to services →</a></nav>`)
	return b.String()
}
