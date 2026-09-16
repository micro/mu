package app

import (
	_ "embed"
	"html"
	"mu/internal/auth"
)

//go:embed console.css
var consoleCSS string

// ConsoleHTML is the single sparse shell, also used by authentication and legal pages.
func ConsoleHTML(title, body string, acc *auth.Account) string {
	links := `<a href="/login">Log in</a><a href="/signup">Sign up</a>`
	if acc != nil {
		links = `<span>` + html.EscapeString(acc.ID) + `</span><a href="/logout">Log out</a>`
	}
	if title != "Micro" {
		title += " | Micro"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="referrer" content="no-referrer"><title>` + html.EscapeString(title) + `</title><link rel="icon" href="/favicon.ico"><style>` + consoleCSS + `</style></head><body><header><a href="/" class="brand">Micro</a><nav aria-label="Account">` + links + `</nav></header><main>` + body + `</main><footer aria-label="Site information">` + FooterLinks() + `</footer></body></html>`
}
