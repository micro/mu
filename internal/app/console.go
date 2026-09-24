package app

import (
	"html"
	"mu/internal/auth"
	"mu/internal/service"
	"net/url"
	"strings"
)

// ConsoleHTML is the single sparse shell, also used by authentication and legal pages.
func ConsoleHTML(title, body string, acc *auth.Account, returnTo ...string) string {
	login := "/login"
	if len(returnTo) > 0 && strings.HasPrefix(returnTo[0], "/") && !strings.HasPrefix(returnTo[0], "//") {
		login += "?redirect=" + url.QueryEscape(returnTo[0])
	}
	links := `<a href="` + html.EscapeString(login) + `">Login</a>`
	if acc != nil {
		name := strings.TrimSpace(acc.Name)
		if name == "" {
			name = strings.TrimSpace(acc.ID)
		}
		initial := "?"
		if letters := []rune(name); len(letters) > 0 {
			initial = strings.ToUpper(string(letters[0]))
		}
		links = `<details class="account-menu"><summary aria-label="Account menu" title="` + html.EscapeString(name) + `"><span aria-hidden="true">` + html.EscapeString(initial) + `</span></summary><div class="account-menu-links"><a href="/account">Account</a>`
		if acc.Admin {
			links += `<a href="/admin">Admin</a>`
		}
		links += `<a href="/logout">Logout</a></div></details>`
	}
	navigation := ""
	if acc != nil {
		here := ""
		if len(returnTo) > 0 {
			here = strings.SplitN(returnTo[0], "?", 2)[0]
		}
		navigation = runtimeNavigation(here)
	}
	if title != "Micro" {
		title += " | Micro"
	}
	pageClass := "document-page"
	if title == "Micro" || (strings.Contains(body, `class="assistant-workspace"`) && !strings.Contains(body, `data-home-overview`)) {
		pageClass = "command-page"
	}
	if title == "Log in | Micro" || title == "Sign up | Micro" {
		pageClass = "auth-page"
	}
	if acc != nil {
		pageClass += " signed-in"
	}
	footer := ""
	if acc == nil {
		footer = `<footer aria-label="Site information">` + strings.ReplaceAll(FooterLinks(), " · ", "") + `</footer>`
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover"><meta name="apple-mobile-web-app-title" content="Micro"><meta name="application-name" content="Micro"><link rel="manifest" href="/manifest.webmanifest"><meta name="referrer" content="no-referrer"><title>` + html.EscapeString(title) + `</title><link rel="icon" href="/favicon.ico"><link rel="stylesheet" href="/mu.css?v=layout-66"><script defer src="/mu.js?v=prompt-64"></script><link rel="apple-touch-icon" href="/icon-192.png"><meta name="theme-color" content="#ffffff"></head><body class="` + pageClass + `">` + navigation + `<div class="page"><header><a href="/" class="brand">Micro</a><nav class="desktop-navigation" aria-label="Navigation">` + links + `</nav></header><main>` + body + `</main></div>` + footer + `</body></html>`
}

// The product destinations stay the same across screen sizes. Services retain
// their own URLs and authorization; navigation only identifies their section.
func runtimeNavigation(path string) string {
	active := ""
	switch {
	case path == "/home" || strings.HasPrefix(path, "/home/"):
		active = "Home"
	case path == "/inbox" || strings.HasPrefix(path, "/inbox/"):
		active = "Inbox"
	case path == "/agents" || path == "/agent" || strings.HasPrefix(path, "/agent/"):
		active = "Agents"
	case path == "/work" || strings.HasPrefix(path, "/work/"):
		active = "Work"
	case path == "/services" || strings.HasPrefix(path, "/services/") || path == "/tools":
		active = "Services"
	default:
		for _, spec := range service.Specs() {
			if spec.Page != "" && (path == spec.Page || strings.HasPrefix(path, spec.Page+"/")) {
				active = "Services"
				break
			}
		}
	}
	var b strings.Builder
	b.WriteString(`<nav class="runtime-navigation" aria-label="Main navigation">`)
	for _, item := range []struct{ path, label, shape string }{
		{"/home", "Home", `M3 10 12 3l9 7M5 9v12h5v-7h4v7h5V9`},
		{"/inbox", "Inbox", `M4 4h16l2 11v5H2v-5L4 4ZM2 15h6l2 3h4l2-3h6`},
		{"/agents", "Agents", `M4 4h16v12H9l-5 4V4ZM8 8h8M8 12h5`},
		{"/work", "Work", `M8 6V3h8v3M3 7h18v14H3V7ZM3 12h18M10 12v3h4v-3`},
		{"/services", "Services", `M3 3h7v7H3V3ZM14 3h7v7h-7V3ZM3 14h7v7H3v-7ZM14 14h7v7h-7v-7Z`},
	} {
		current := ""
		if active == item.label {
			current = ` aria-current="page"`
		}
		b.WriteString(`<a href="` + item.path + `"` + current + `><svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="` + item.shape + `"/></svg><span>` + item.label + `</span></a>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}
