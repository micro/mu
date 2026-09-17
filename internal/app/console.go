package app

import (
	"html"
	"mu/internal/auth"
	"strings"
)

// ConsoleHTML is the single sparse shell, also used by authentication and legal pages.
func ConsoleHTML(title, body string, acc *auth.Account) string {
	links := `<a href="/">Home</a><a href="/login">Login</a>`
	if acc != nil {
		links = `<a href="/">Home</a><a href="/inbox">Inbox</a><a href="/account">Account</a>`
		if acc.Admin {
			links += `<a href="/admin">Admin</a>`
		}
		links += `<a href="/logout">Logout</a>`
	}
	if title != "Micro" {
		title += " | Micro"
	}
	pageClass := "document-page"
	if title == "Micro" {
		pageClass = "command-page"
	}
	if title == "Log in | Micro" || title == "Sign up | Micro" {
		pageClass = "auth-page"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover"><meta name="apple-mobile-web-app-title" content="Micro"><meta name="application-name" content="Micro"><link rel="manifest" href="/manifest.webmanifest"><meta name="referrer" content="no-referrer"><title>` + html.EscapeString(title) + `</title><link rel="icon" href="/favicon.ico"><link rel="stylesheet" href="/mu.css?v=navigation-16"><script defer src="/mu.js?v=navigation-16"></script><link rel="apple-touch-icon" href="/icon-192.png"><meta name="theme-color" content="#ffffff"></head><body class="` + pageClass + `"><div class="page"><header><button type="button" class="nav-toggle" aria-label="Open navigation" aria-controls="mobile-navigation" aria-expanded="false"><svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M3 5h14M3 10h14M3 15h14"/></svg></button><a href="/" class="brand">Micro</a><nav class="desktop-navigation" aria-label="Navigation">` + links + `</nav></header><main>` + body + `</main></div><dialog id="mobile-navigation" class="nav-drawer" aria-label="Navigation"><div class="nav-drawer-heading"><span class="brand">Micro</span><button type="button" class="nav-close" aria-label="Close navigation">×</button></div><nav aria-label="Mobile navigation">` + links + `</nav></dialog><footer aria-label="Site information">` + strings.ReplaceAll(FooterLinks(), " · ", "") + `</footer></body></html>`
}
