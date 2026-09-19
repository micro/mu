package app

import (
	"html"
	"mu/internal/auth"
	"strings"
)

// ConsoleHTML is the single sparse shell, also used by authentication and legal pages.
func ConsoleHTML(title, body string, acc *auth.Account) string {
	links := `<a href="/">Home</a>`
	accountLink := `<a class="mobile-account" href="/login">Login</a>`
	sidebar := ""
	if acc != nil {
		accountLink = ""
		links += `<a href="/logout">Logout</a>`
		secondary := `<a href="/account">Account</a>`
		if acc.Admin {
			secondary += `<a href="/admin">Admin</a>`
		}
		sidebar = `<nav class="sidebar-primary" aria-label="Main navigation"><a href="/inbox">Inbox</a>` + secondary + `<a href="/logout">Logout</a></nav>`
	} else {
		links += `<a href="/login">Login</a>`
		sidebar = `<nav aria-label="Navigation">` + links + `</nav>`
	}
	if title != "Micro" {
		title += " | Micro"
	}
	pageClass := "document-page"
	if title == "Micro" || strings.Contains(body, `class="assistant-workspace"`) {
		pageClass = "command-page"
	}
	if title == "Log in | Micro" || title == "Sign up | Micro" {
		pageClass = "auth-page"
	}
	historyToggle := ""
	if pageClass == "command-page" && acc != nil {
		historyToggle = `<button type="button" class="history-toggle" data-history-toggle aria-label="Toggle sidebar" title="Sidebar" aria-controls="assistant-history" aria-expanded="false"><svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><rect x="2.5" y="3" width="15" height="14" rx="2"/><path d="M7.5 3v14"/></svg></button>`
	}
	if acc != nil {
		pageClass += " signed-in"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover"><meta name="apple-mobile-web-app-title" content="Micro"><meta name="application-name" content="Micro"><link rel="manifest" href="/manifest.webmanifest"><meta name="referrer" content="no-referrer"><title>` + html.EscapeString(title) + `</title><link rel="icon" href="/favicon.ico"><link rel="stylesheet" href="/mu.css?v=layout-45"><script defer src="/mu.js?v=prompt-51"></script><link rel="apple-touch-icon" href="/icon-192.png"><meta name="theme-color" content="#ffffff"></head><body class="` + pageClass + `"><div class="page"><header>` + historyToggle + `<button type="button" class="nav-toggle" data-nav-toggle aria-label="Open navigation" aria-controls="mobile-navigation" aria-expanded="false"><svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><rect x="2.5" y="3" width="15" height="14" rx="2"/><path d="M7.5 3v14"/></svg></button><a href="/" class="brand">Micro</a><nav class="desktop-navigation" aria-label="Navigation">` + links + `</nav>` + accountLink + `</header><main>` + body + `</main></div><dialog id="mobile-navigation" class="nav-drawer" data-nav-drawer aria-label="Navigation"><div class="nav-drawer-heading"><span class="brand">Micro</span><button type="button" class="nav-close" data-nav-close aria-label="Close navigation">×</button></div>` + sidebar + `</dialog><footer aria-label="Site information">` + strings.ReplaceAll(FooterLinks(), " · ", "") + `</footer></body></html>`
}
