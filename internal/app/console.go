package app

import (
	"html"
	"mu/internal/auth"
	"strings"
)

// ConsoleHTML is the single sparse shell, also used by authentication and legal pages.
func ConsoleHTML(title, body string, acc *auth.Account) string {
	links := `<a href="/">Home</a>`
	if acc != nil {
		links += `<a href="/account">Account</a>`
		if acc.Admin {
			links += `<a href="/admin">Admin</a>`
		}
		links += `<a href="/inbox">Inbox</a><a href="/logout">Logout</a>`
	} else {
		links += `<a href="/login">Login</a>`
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
	if acc != nil {
		pageClass += " signed-in"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover"><meta name="apple-mobile-web-app-title" content="Micro"><meta name="application-name" content="Micro"><link rel="manifest" href="/manifest.webmanifest"><meta name="referrer" content="no-referrer"><title>` + html.EscapeString(title) + `</title><link rel="icon" href="/favicon.ico"><link rel="stylesheet" href="/mu.css?v=layout-50"><script defer src="/mu.js?v=prompt-55"></script><link rel="apple-touch-icon" href="/icon-192.png"><meta name="theme-color" content="#ffffff"></head><body class="` + pageClass + `"><div class="page"><header><a href="/" class="brand">Micro</a><nav class="desktop-navigation" aria-label="Navigation">` + links + `</nav></header><main>` + body + `</main></div><footer aria-label="Site information">` + strings.ReplaceAll(FooterLinks(), " · ", "") + `</footer></body></html>`
}
